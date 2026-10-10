package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"caddy-panel/backend/internal/config"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

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
		},
		Store:  st,
		Bundle: bundle,
	})
	return engine, st
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
