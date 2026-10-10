package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// fakeSiteRepo 内存版 SiteRepo。
type fakeSiteRepo struct {
	mu       sync.Mutex
	sites    map[uint]*model.Site
	nextID   uint
	versions []model.ConfigVersion
	settings map[string]string
}

func newFakeSiteRepo() *fakeSiteRepo {
	return &fakeSiteRepo{sites: map[uint]*model.Site{}, nextID: 1, settings: map[string]string{}}
}

func (f *fakeSiteRepo) CreateSite(_ context.Context, site *model.Site) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	site.ID = f.nextID
	f.nextID++
	cp := *site
	f.sites[site.ID] = &cp
	return nil
}

func (f *fakeSiteRepo) GetSite(_ context.Context, id uint) (*model.Site, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	site, ok := f.sites[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *site
	return &cp, nil
}

func (f *fakeSiteRepo) ListSites(_ context.Context) ([]model.Site, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.Site, 0, len(f.sites))
	for id := 1; uint(id) < f.nextID; id++ {
		if s, ok := f.sites[uint(id)]; ok {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (f *fakeSiteRepo) UpdateSite(_ context.Context, site *model.Site) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sites[site.ID]; !ok {
		return store.ErrNotFound
	}
	cp := *site
	f.sites[site.ID] = &cp
	return nil
}

func (f *fakeSiteRepo) DeleteSite(_ context.Context, id uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sites, id)
	return nil
}

func (f *fakeSiteRepo) CreateConfigVersion(_ context.Context, version *model.ConfigVersion) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versions = append(f.versions, *version)
	return nil
}

func (f *fakeSiteRepo) GetSetting(_ context.Context, key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.settings[key]
	return v, ok, nil
}

func (f *fakeSiteRepo) SetSetting(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings[key] = value
	return nil
}

func (f *fakeSiteRepo) ReplaceSettings(_ context.Context, settings []model.Setting) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings = map[string]string{}
	for _, s := range settings {
		f.settings[s.Key] = s.Value
	}
	return nil
}

// fakeRenderInput 捕获 pipeline 收到的渲染输入,供断言全量语义。
var lastRenderInput caddy.RenderInput

type captureRenderer struct{ out string }

func (f *captureRenderer) Render(input caddy.RenderInput) (string, error) {
	lastRenderInput = input
	return f.out, nil
}

type passValidator struct{ err error }

func (f *passValidator) Validate(context.Context, string) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`{"apps":{}}`), nil
}

type recordingClient struct {
	loadCalls int
}

func (f *recordingClient) GetConfig(context.Context) (json.RawMessage, string, error) {
	return json.RawMessage(`{}`), `"e"`, nil
}

func (f *recordingClient) Load(context.Context, json.RawMessage, string) error {
	f.loadCalls++
	return nil
}

func (f *recordingClient) Adapt(context.Context, string) (json.RawMessage, error) {
	return nil, nil
}

func (f *recordingClient) Upstreams(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (f *recordingClient) Ping(ctx context.Context) error {
	_, _, err := f.GetConfig(ctx)
	return err
}

// newSiteFixture 组装 service 与各 fake。
func newSiteFixture(t *testing.T, validatorErr error) (*Site, *fakeSiteRepo, *recordingClient) {
	t.Helper()
	repo := newFakeSiteRepo()
	client := &recordingClient{}
	pipeline := caddy.NewPipeline(&captureRenderer{out: "rendered"},
		&passValidator{err: validatorErr}, client, func() string { return "/tmp/unused-Caddyfile" })
	svc := NewSite(SiteDeps{
		Repo:     repo,
		Pipeline: pipeline,
		Defaults: SiteDefaults{AdminBind: "localhost:2019", ErrorLogPath: "/e.log", AccessLogPath: "/a.log"},
	})
	return svc, repo, client
}

func proxyInput() SiteInput {
	return SiteInput{
		Name: "app", Type: model.SiteTypeProxy,
		Domains: "app.example.com", Upstreams: "127.0.0.1:8080",
		HTTPSRedirect: true, Enabled: true,
	}
}

func TestSiteCreate_DeploysAndPersists(t *testing.T) {
	svc, repo, client := newSiteFixture(t, nil)

	site, result, err := svc.Create(context.Background(), proxyInput(), false)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if site.ID == 0 || !result.Applied {
		t.Fatalf("结果不符: site=%+v applied=%v", site.ID, result.Applied)
	}
	if client.loadCalls != 1 {
		t.Fatalf("应下发一次,实际 %d", client.loadCalls)
	}
	if _, ok := repo.sites[site.ID]; !ok {
		t.Fatal("站点应已落库")
	}
	if len(repo.versions) != 1 || repo.versions[0].Status != model.VersionStatusApplied {
		t.Fatalf("应有一条 applied 快照: %+v", repo.versions)
	}
	// 全量渲染输入应包含新站点。
	if len(lastRenderInput.Sites) != 1 || lastRenderInput.Sites[0].Domains != "app.example.com" {
		t.Fatalf("渲染输入应含新站点: %+v", lastRenderInput.Sites)
	}
}

func TestSiteCreate_DryRunPersistsNothing(t *testing.T) {
	svc, repo, client := newSiteFixture(t, nil)

	site, result, err := svc.Create(context.Background(), proxyInput(), true)
	if err != nil {
		t.Fatalf("dry run 失败: %v", err)
	}
	if site.ID != 0 || result.Applied {
		t.Fatalf("dry run 不应落库: id=%d applied=%v", site.ID, result.Applied)
	}
	if client.loadCalls != 0 || len(repo.sites) != 0 || len(repo.versions) != 0 {
		t.Fatal("dry run 不应触 Caddy/落库/写快照")
	}
}

func TestSiteCreate_InvalidInputRejected(t *testing.T) {
	svc, repo, client := newSiteFixture(t, nil)

	in := proxyInput()
	in.Domains = ""
	if _, _, err := svc.Create(context.Background(), in, false); err == nil {
		t.Fatal("空域名应被拦截")
	}
	if len(repo.sites) != 0 || client.loadCalls != 0 {
		t.Fatal("非法输入不应有任何副作用")
	}
}

func TestSiteCreate_DomainConflict(t *testing.T) {
	svc, _, _ := newSiteFixture(t, nil)
	ctx := context.Background()

	if _, _, err := svc.Create(ctx, proxyInput(), false); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	in := proxyInput()
	in.Name = "another"
	var conflict *DomainConflictError
	if _, _, err := svc.Create(ctx, in, false); !errors.As(err, &conflict) {
		t.Fatalf("同域名应报冲突: %v", err)
	}
}

func TestSiteCreate_DeployFails_RecordsFailedVersionOnly(t *testing.T) {
	svc, repo, client := newSiteFixture(t, errors.New("Caddyfile 校验失败: bad directive"))

	_, _, err := svc.Create(context.Background(), proxyInput(), false)
	var deploy *DeployError
	if !errors.As(err, &deploy) {
		t.Fatalf("应返回 DeployError: %v", err)
	}
	if len(repo.sites) != 0 {
		t.Fatal("下发失败不应落库站点")
	}
	if len(repo.versions) != 1 || repo.versions[0].Status != model.VersionStatusFailed {
		t.Fatalf("应留 failed 快照: %+v", repo.versions)
	}
	if client.loadCalls != 0 {
		t.Fatal("校验失败不应下发")
	}
}

func TestSiteUpdate_RendersNewConfig(t *testing.T) {
	svc, repo, client := newSiteFixture(t, nil)
	ctx := context.Background()

	site, _, err := svc.Create(ctx, proxyInput(), false)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	in := proxyInput()
	in.Domains = "new.example.com"
	updated, _, err := svc.Update(ctx, site.ID, in)
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Domains != "new.example.com" || updated.ID != site.ID {
		t.Fatalf("更新结果不符: %+v", updated)
	}
	if repo.sites[site.ID].Domains != "new.example.com" {
		t.Fatal("库内站点应已更新")
	}
	if client.loadCalls != 2 {
		t.Fatalf("创建+更新共两次下发,实际 %d", client.loadCalls)
	}
	if len(lastRenderInput.Sites) != 1 || lastRenderInput.Sites[0].Domains != "new.example.com" {
		t.Fatalf("渲染输入应含新配置: %+v", lastRenderInput.Sites)
	}
}

func TestSiteSetEnabled_TogglesOff(t *testing.T) {
	svc, repo, _ := newSiteFixture(t, nil)
	ctx := context.Background()

	site, _, err := svc.Create(ctx, proxyInput(), false)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if _, _, err := svc.SetEnabled(ctx, site.ID, false); err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if repo.sites[site.ID].Enabled {
		t.Fatal("库内站点应为停用")
	}
}

func TestSiteDelete_RemovesSite(t *testing.T) {
	svc, repo, client := newSiteFixture(t, nil)
	ctx := context.Background()

	site, _, err := svc.Create(ctx, proxyInput(), false)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if _, err := svc.Delete(ctx, site.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if len(repo.sites) != 0 {
		t.Fatal("站点应已删除")
	}
	if client.loadCalls != 2 {
		t.Fatalf("删除需要一次下发(空配置),实际总下发 %d", client.loadCalls)
	}
	if len(lastRenderInput.Sites) != 0 {
		t.Fatalf("删除后渲染输入不应含该站点: %+v", lastRenderInput.Sites)
	}
}

func TestSiteGet_NotFound(t *testing.T) {
	svc, _, _ := newSiteFixture(t, nil)
	if _, err := svc.Get(context.Background(), 999); !errors.Is(err, ErrSiteNotFound) {
		t.Fatalf("期望 ErrSiteNotFound: %v", err)
	}
}

func TestSiteCreate_BasicAuthStoredAsHash(t *testing.T) {
	svc, repo, _ := newSiteFixture(t, nil)

	in := proxyInput()
	in.BasicAuth = []BasicAuthUser{{Username: "alice", Password: "secret123"}}
	in.IPBlocklist = "10.0.0.0/8, 192.168.1.1"

	site, _, err := svc.Create(context.Background(), in, false)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	stored := repo.sites[site.ID]
	if stored.BasicAuthUsers == "" || strings.Contains(stored.BasicAuthUsers, "secret123") {
		t.Fatalf("落库必须是 bcrypt 哈希且不含明文: %s", stored.BasicAuthUsers)
	}
	var entries []struct {
		Username     string `json:"username"`
		PasswordHash string `json:"password_hash"`
	}
	if err := json.Unmarshal([]byte(stored.BasicAuthUsers), &entries); err != nil {
		t.Fatalf("哈希 JSON 解析失败: %v", err)
	}
	if len(entries) != 1 || entries[0].Username != "alice" || !strings.HasPrefix(entries[0].PasswordHash, "$2") {
		t.Fatalf("哈希条目不符: %+v", entries)
	}
	if stored.IPBlocklist != "10.0.0.0/8, 192.168.1.1" {
		t.Fatalf("IP 黑名单未落库: %q", stored.IPBlocklist)
	}
}

func TestSiteInput_BasicAuthValidation(t *testing.T) {
	in := proxyInput()
	in.BasicAuth = []BasicAuthUser{{Username: "dup", Password: "aaaa"}, {Username: "dup", Password: "bbbb"}}
	if err := in.Validate(); err == nil {
		t.Fatal("重复用户名应被拒绝")
	}
	in.BasicAuth = []BasicAuthUser{{Username: "u", Password: "abc"}}
	if err := in.Validate(); err == nil {
		t.Fatal("过短密码应被拒绝")
	}
	in.IPAllowlist = "not-an-ip"
	if err := in.Validate(); err == nil {
		t.Fatal("非法 IP 应被拒绝")
	}
}
