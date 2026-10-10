package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/config"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// fakeDeployClient 站点集成测试用:Caddy 侧全部放行。
type fakeDeployClient struct{}

func (fakeDeployClient) Ping(context.Context) error { return nil }

func (fakeDeployClient) Adapt(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (fakeDeployClient) Load(context.Context, json.RawMessage, string) error { return nil }

func (fakeDeployClient) GetConfig(context.Context) (json.RawMessage, string, error) {
	return json.RawMessage(`{}`), `"etag"`, nil
}

// newTestServer 起真实路由 + 临时 SQLite,覆盖 handler→service→store 全链路。
func newTestServer(t *testing.T) (*gin.Engine, *store.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	hash, err := bcrypt.GenerateFromPassword([]byte("right-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("哈希测试密码: %v", err)
	}
	if err := st.CreateUser(t.Context(), &model.User{Username: "admin", PasswordHash: string(hash), Role: "admin"}); err != nil {
		t.Fatalf("创建测试用户: %v", err)
	}

	bundle, err := i18n.New()
	if err != nil {
		t.Fatalf("初始化 i18n: %v", err)
	}

	engine := New(Deps{
		Cfg: &config.Config{
			Mode:            gin.TestMode,
			JWTSecret:       "test-secret",
			JWTTTL:          time.Hour,
			LoginMaxFails:   5,
			LoginLockWindow: time.Minute,
			CaddyAdminBind:  "localhost:2019",
			LogErrorPath:    "/tmp/test-error.log",
			LogAccessPath:   "/tmp/test-access.log",
		},
		Store:  st,
		Bundle: bundle,
		Pipeline: caddy.NewPipeline(
			caddy.NewRenderer(),
			caddy.NewValidator(fakeDeployClient{}),
			fakeDeployClient{},
			func() string { return filepath.Join(t.TempDir(), "Caddyfile") },
		),
	})
	return engine, st
}

// loginToken 登录测试用户并返回 JWT。
func loginToken(t *testing.T, engine *gin.Engine) string {
	t.Helper()
	rec := postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "right-password"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("测试登录失败: %d %s", rec.Code, rec.Body.String())
	}
	return decode(t, rec)["token"].(string)
}

func authHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func postJSON(t *testing.T, engine *gin.Engine, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化请求体: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func getJSON(t *testing.T, engine *gin.Engine, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// decode 便捷解码响应体。
func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应 %s: %v", rec.Body.String(), err)
	}
	return body
}

func TestLoginFlow(t *testing.T) {
	engine, _ := newTestServer(t)

	// 错误凭证:401 + 统一错误结构。
	rec := postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "wrong"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401,实际 %d: %s", rec.Code, rec.Body.String())
	}
	if body := decode(t, rec); body["code"] != "AUTH_INVALID_CREDENTIALS" {
		t.Fatalf("错误码不符: %v", body)
	}

	// 正确凭证:200 + token。
	rec = postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "right-password"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200,实际 %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("token 为空: %v", body)
	}

	// /me 带 token:返回用户身份。
	rec = getJSON(t, engine, "/api/v1/me", map[string]string{"Authorization": "Bearer " + token})
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200,实际 %d: %s", rec.Code, rec.Body.String())
	}
	if me := decode(t, rec); me["username"] != "admin" {
		t.Fatalf("/me 返回不符: %v", me)
	}

	// /me 无 token:401。
	rec = getJSON(t, engine, "/api/v1/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 期望 401,实际 %d", rec.Code)
	}
	if body := decode(t, rec); body["code"] != "AUTH_UNAUTHORIZED" {
		t.Fatalf("未授权错误码不符: %v", body)
	}

	// /me 带伪造 token:401。
	rec = getJSON(t, engine, "/api/v1/me", map[string]string{"Authorization": "Bearer fake.token.here"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("伪造 token 期望 401,实际 %d", rec.Code)
	}
}

func TestLogin_ErrorMessagesFollowAcceptLanguage(t *testing.T) {
	engine, _ := newTestServer(t)

	rec := postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "wrong"},
		map[string]string{"Accept-Language": "en-US"})
	body := decode(t, rec)
	if body["message"] != "Incorrect username or password" {
		t.Fatalf("英文错误消息不符: %v", body)
	}

	rec = postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "wrong"},
		map[string]string{"Accept-Language": "zh-CN"})
	body = decode(t, rec)
	if body["message"] != "用户名或密码错误" {
		t.Fatalf("中文错误消息不符: %v", body)
	}
}

func TestLogin_SourceLocksAcrossRequests(t *testing.T) {
	engine, _ := newTestServer(t)
	// httptest 请求同源(127.0.0.1):连续失败 4 次仍为凭证错误,
	// 第 5 次(达到阈值)即返回 429,之后正确密码也被拒。
	for i := 0; i < 4; i++ {
		rec := postJSON(t, engine, "/api/v1/auth/login",
			map[string]string{"username": "admin", "password": "wrong"}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次失败期望 401,实际 %d", i+1, rec.Code)
		}
	}
	rec := postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "wrong"}, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("第 5 次失败(达阈值)期望 429,实际 %d: %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "right-password"}, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("锁定期间期望 429,实际 %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("锁定响应应携带 Retry-After 头")
	}
}

func TestLogin_WritesAuditLog(t *testing.T) {
	engine, st := newTestServer(t)

	postJSON(t, engine, "/api/v1/auth/login",
		map[string]string{"username": "admin", "password": "right-password"}, nil)

	var logs []model.AuditLog
	if err := st.DB().Find(&logs).Error; err != nil {
		t.Fatalf("查询审计表: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("期望 1 条审计记录,实际 %d", len(logs))
	}
	if logs[0].Action != "auth.login" || logs[0].UserID == nil {
		t.Fatalf("审计记录不符: %+v", logs[0])
	}
}

// siteBody 站点写接口的合法请求体。
func siteBody(name string) map[string]any {
	return map[string]any{
		"name": name, "type": "proxy",
		"domains": name + ".example.com", "upstreams": "127.0.0.1:8080",
		"https_redirect": true, "enabled": true,
	}
}

func TestSiteCRUD_Flow(t *testing.T) {
	engine, st := newTestServer(t)
	token := loginToken(t, engine)
	h := authHeader(token)

	// 未登录访问站点列表:401。
	if rec := getJSON(t, engine, "/api/v1/sites", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录期望 401,实际 %d", rec.Code)
	}

	// dry run:预检通过但不落库。
	rec := postJSON(t, engine, "/api/v1/sites?dry_run=true", siteBody("dry"), h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("dry run 期望 201,实际 %d: %s", rec.Code, rec.Body.String())
	}
	if body := decode(t, rec); body["applied"] != false {
		t.Fatalf("dry run applied 应为 false: %v", body)
	}
	var count int64
	st.DB().Model(&model.Site{}).Count(&count)
	if count != 0 {
		t.Fatalf("dry run 不应落库,实际 %d 条", count)
	}

	// 正常创建。
	rec = postJSON(t, engine, "/api/v1/sites", siteBody("app"), h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建期望 201,实际 %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	siteMap := body["site"].(map[string]any)
	siteID := uint(siteMap["id"].(float64))
	if body["applied"] != true {
		t.Fatal("创建后 applied 应为 true")
	}

	// 列表与详情。
	if rec := getJSON(t, engine, "/api/v1/sites", h); rec.Code != http.StatusOK {
		t.Fatalf("列表期望 200,实际 %d", rec.Code)
	}
	if rec := getJSON(t, engine, "/api/v1/sites/"+strconvFormat(uint64(siteID)), h); rec.Code != http.StatusOK {
		t.Fatalf("详情期望 200,实际 %d", rec.Code)
	}

	// 更新。
	updateBody := siteBody("app")
	updateBody["domains"] = "updated.example.com"
	req := httptest.NewRequest(http.MethodPut, "/api/v1/sites/"+strconvFormat(uint64(siteID)), marshal(t, updateBody))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range h {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新期望 200,实际 %d: %s", rec.Code, rec.Body.String())
	}

	// 停用。
	req = httptest.NewRequest(http.MethodPost, "/api/v1/sites/"+strconvFormat(uint64(siteID))+"/disable", nil)
	for k, v := range h {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("停用期望 200,实际 %d: %s", rec.Code, rec.Body.String())
	}

	// 删除。
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/sites/"+strconvFormat(uint64(siteID)), nil)
	for k, v := range h {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除期望 204,实际 %d: %s", rec.Code, rec.Body.String())
	}
	st.DB().Model(&model.Site{}).Count(&count)
	if count != 0 {
		t.Fatalf("删除后应无站点,实际 %d", count)
	}

	// 版本快照:dry run 不留痕,创建/更新/停用/删除各一条 applied。
	st.DB().Model(&model.ConfigVersion{}).Where("status = ?", model.VersionStatusApplied).Count(&count)
	if count != 4 {
		t.Fatalf("期望 4 条 applied 快照,实际 %d", count)
	}
}

func TestSite_DomainConflictAndInvalidInput(t *testing.T) {
	engine, _ := newTestServer(t)
	token := loginToken(t, engine)
	h := authHeader(token)

	if rec := postJSON(t, engine, "/api/v1/sites", siteBody("app"), h); rec.Code != http.StatusCreated {
		t.Fatalf("首个站点创建失败: %d %s", rec.Code, rec.Body.String())
	}

	// 同域名:409(与首个站点相同的域名)。
	dup := siteBody("dup")
	dup["domains"] = "app.example.com"
	rec := postJSON(t, engine, "/api/v1/sites", dup, h)
	if rec.Code != http.StatusConflict {
		t.Fatalf("同域名期望 409,实际 %d: %s", rec.Code, rec.Body.String())
	}
	if body := decode(t, rec); body["code"] != "SITE_DOMAIN_CONFLICT" {
		t.Fatalf("错误码不符: %v", body)
	}

	// 非法上游:400。
	bad := siteBody("bad")
	bad["upstreams"] = "no-port-here"
	rec = postJSON(t, engine, "/api/v1/sites", bad, h)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法上游期望 400,实际 %d: %s", rec.Code, rec.Body.String())
	}

	// 不存在的站点:404。
	if rec := getJSON(t, engine, "/api/v1/sites/999", h); rec.Code != http.StatusNotFound {
		t.Fatalf("不存在站点期望 404,实际 %d", rec.Code)
	}
}

func marshal(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	return bytes.NewReader(raw)
}

func strconvFormat(v uint64) string {
	return json.Number(uintToString(v)).String()
}

func uintToString(v uint64) string {
	if v == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	return string(digits[i:])
}
