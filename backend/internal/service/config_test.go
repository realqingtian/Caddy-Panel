package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// listVersionsPtr 版本自增 ID(测试内手动分配)。
var versionSeq uint

// resetVersions 清空 fakeSiteRepo 的版本表并重置序号。
func (f *fakeSiteRepo) resetVersions() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versions = nil
	versionSeq = 0
}

func (f *fakeSiteRepo) ListConfigVersions(context.Context) ([]model.ConfigVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.ConfigVersion, 0, len(f.versions))
	for i := len(f.versions) - 1; i >= 0; i-- {
		out = append(out, f.versions[i])
	}
	return out, nil
}

func (f *fakeSiteRepo) GetConfigVersion(_ context.Context, id uint) (*model.ConfigVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.versions {
		if v.ID == id {
			cp := v
			return &cp, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeSiteRepo) ReplaceConfigVersions(_ context.Context, versions []model.ConfigVersion) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versions = nil
	versionSeq = 0
	for i := range versions {
		cp := versions[i]
		versionSeq++
		cp.ID = versionSeq
		f.versions = append(f.versions, cp)
	}
	return nil
}

func (f *fakeSiteRepo) ReplaceAllSites(_ context.Context, sites []model.Site) error {
	return f.replaceNodeSitesImpl(sites)
}

func (f *fakeSiteRepo) ReplaceNodeSites(ctx context.Context, nodeID string, sites []model.Site) error {
	return f.replaceNodeSitesImpl(sites)
}

func (f *fakeSiteRepo) replaceNodeSitesImpl(sites []model.Site) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sites = map[uint]*model.Site{}
	for i := range sites {
		cp := sites[i]
		f.sites[cp.ID] = &cp
	}
	return nil
}

// recordVersionForTest 直接写入一条版本(绕过 service,构造历史数据)。
func (f *fakeSiteRepo) recordVersionForTest(rendered string, status string, snapshot string) uint {
	f.mu.Lock()
	defer f.mu.Unlock()
	versionSeq++
	f.versions = append(f.versions, model.ConfigVersion{
		ID: versionSeq, SitesSnapshot: snapshot, RenderedCaddyfile: rendered, Status: status,
	})
	return versionSeq
}

func newConfigFixture(t *testing.T, validatorErr error) (*Config, *fakeSiteRepo, *recordingClient) {
	t.Helper()
	repo := newFakeSiteRepo()
	client := &recordingClient{}
	pipeline := caddy.NewPipeline(&captureRenderer{out: "rendered"},
		&passValidator{err: validatorErr}, localApplierFor(client))
	svc := NewConfig(ConfigDeps{Repo: repo, Pipeline: pipeline, CaddyfilePathFn: func() string { return "/tmp/unused" }})
	return svc, repo, client
}

func sitesSnapshotJSON(sites []model.Site) string {
	data, _ := json.Marshal(sites)
	return string(data)
}

func TestConfigRollback_RestoresSnapshotAndDeploys(t *testing.T) {
	svc, repo, client := newConfigFixture(t, nil)
	ctx := context.Background()

	// 历史:站点 A 上线(版本1)→ 改名为 B(版本2)。
	oldSites := []model.Site{{ID: 1, Name: "app", Type: model.SiteTypeProxy, Domains: "old.example.com", Enabled: true}}
	v1 := repo.recordVersionForTest("old-caddyfile", model.VersionStatusApplied, sitesSnapshotJSON(oldSites))
	_ = repo.CreateSite(ctx, &model.Site{ID: 1, Name: "app", Type: model.SiteTypeProxy, Domains: "new.example.com", Enabled: true})
	_ = repo.recordVersionForTest("new-caddyfile", model.VersionStatusApplied, sitesSnapshotJSON([]model.Site{{ID: 1, Name: "app", Domains: "new.example.com"}}))
	loadCallsBefore := client.loadCalls

	applied, err := svc.Rollback(ctx, v1)
	if err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	if applied.Status != model.VersionStatusApplied {
		t.Fatalf("回滚应生成新 applied 版本: %+v", applied)
	}
	if client.loadCalls != loadCallsBefore+1 {
		t.Fatalf("回滚应下发一次,实际 %d", client.loadCalls-loadCallsBefore)
	}
	// 站点表恢复为快照内容。
	restored, _ := repo.GetSite(ctx, 1)
	if restored.Domains != "old.example.com" {
		t.Fatalf("站点应恢复为快照域名,实际 %s", restored.Domains)
	}
}

func TestConfigRollback_RejectsFailedVersion(t *testing.T) {
	svc, repo, _ := newConfigFixture(t, nil)
	_ = repo
	id := repo.recordVersionForTest("", model.VersionStatusFailed, "[]")

	if _, err := svc.Rollback(context.Background(), id); !errors.Is(err, ErrVersionNotApplicable) {
		t.Fatalf("失败版本应拒绝回滚: %v", err)
	}
}

func TestConfigRollback_DeployFails_RecordsFailed(t *testing.T) {
	svc, repo, client := newConfigFixture(t, errors.New("Caddyfile 校验失败: broken"))
	_ = repo
	id := repo.recordVersionForTest("bad-caddyfile", model.VersionStatusApplied, "[]")

	_, err := svc.Rollback(context.Background(), id)
	var deploy *DeployError
	if !errors.As(err, &deploy) {
		t.Fatalf("应返回 DeployError: %v", err)
	}
	if len(repo.versions) != 2 || repo.versions[1].Status != model.VersionStatusFailed {
		t.Fatalf("应留 failed 快痕: %+v", repo.versions)
	}
	if client.loadCalls != 0 {
		t.Fatal("校验失败不应下发")
	}
}

func TestConfigSaveCaddyfile_AppliesAndRecords(t *testing.T) {
	svc, repo, client := newConfigFixture(t, nil)

	version, err := svc.SaveCaddyfile(context.Background(), "{\n\tadmin localhost:2019\n}\nhand-edited")
	if err != nil {
		t.Fatalf("直编失败: %v", err)
	}
	if version.Status != model.VersionStatusApplied {
		t.Fatalf("直编版本不符: %+v", version)
	}
	if len(repo.versions) != 1 || repo.versions[0].Status != model.VersionStatusApplied {
		t.Fatalf("直编应留 applied 快痕: %+v", repo.versions)
	}
	if client.loadCalls != 1 {
		t.Fatalf("直编应下发一次,实际 %d", client.loadCalls)
	}
}

func TestConfigSaveCaddyfile_InvalidRecordsFailed(t *testing.T) {
	svc, repo, _ := newConfigFixture(t, errors.New("Caddyfile 校验失败: syntax"))

	_, err := svc.SaveCaddyfile(context.Background(), "broken")
	if err == nil {
		t.Fatal("非法内容应失败")
	}
	// 缺全局 admin 指令:直接拒绝(防面板失联)。
	if _, err := svc.SaveCaddyfile(context.Background(), "demo.local {\n\trespond \"ok\"\n}"); err == nil {
		t.Fatal("缺 admin 指令的直编应被拒绝")
	}
	// 含 admin 但语法非法:走到校验器,失败并留 failed 快痕。
	_, err = svc.SaveCaddyfile(context.Background(), "{\n\tadmin localhost:2019\n}\nbroken-content")
	if err == nil {
		t.Fatal("非法 Caddyfile 应失败")
	}
	if len(repo.versions) != 1 || repo.versions[0].Status != model.VersionStatusFailed {
		t.Fatalf("应留 failed 快痕: %+v", repo.versions)
	}
}

func TestSettingUpdate_ValidatesAndPersists(t *testing.T) {
	repo := newFakeSiteRepo()
	svc := NewSetting(repo, map[string]string{model.SettingKeyLoginMaxFails: "5"})

	// 非法值拒绝。
	if err := svc.Update(context.Background(), map[string]string{model.SettingKeyLoginMaxFails: "zero"}); err == nil {
		t.Fatal("非法阈值应被拒绝")
	}
	// 未知键拒绝。
	if err := svc.Update(context.Background(), map[string]string{"jwt_secret": "x"}); err == nil {
		t.Fatal("白名单外键应被拒绝")
	}
	// 合法值写入。
	if err := svc.Update(context.Background(), map[string]string{model.SettingKeyLoginMaxFails: "3"}); err != nil {
		t.Fatalf("合法更新失败: %v", err)
	}
	if v, _, _ := repo.GetSetting(context.Background(), model.SettingKeyLoginMaxFails); v != "3" {
		t.Fatalf("值未持久化: %q", v)
	}
}

func TestSettingUpdate_NotificationKeys(t *testing.T) {
	repo := newFakeSiteRepo()
	svc := NewSetting(repo, map[string]string{})

	// 通知键在白名单内,合法值写入(I3-4:此前白名单漏登记,PUT 一律 400)。
	if err := svc.Update(context.Background(), map[string]string{
		model.SettingKeyAcmeEmail: "ops@example.com",
		model.SettingKeySMTPPort:  "465",
	}); err != nil {
		t.Fatalf("通知键合法更新失败: %v", err)
	}
	if v, _, _ := repo.GetSetting(context.Background(), model.SettingKeySMTPPort); v != "465" {
		t.Fatalf("SMTP 端口未持久化: %q", v)
	}
	// 可空键空值跳过格式校验(存空串 = 回落 env 默认)。
	if err := svc.Update(context.Background(), map[string]string{
		model.SettingKeyAcmeEmail: "",
		model.SettingKeySMTPTo:    "",
	}); err != nil {
		t.Fatalf("可空键空值应放行: %v", err)
	}
	// 非空仍校验格式。
	if err := svc.Update(context.Background(), map[string]string{model.SettingKeySMTPPort: "abc"}); err == nil {
		t.Fatal("SMTP 端口非法值应被拒绝")
	}
}

func TestSettingAll_MergesDefaults(t *testing.T) {
	repo := newFakeSiteRepo()
	svc := NewSetting(repo, map[string]string{model.SettingKeyLoginMaxFails: "5", model.SettingKeyAdminURL: "http://localhost:2019"})
	_ = repo.SetSetting(context.Background(), model.SettingKeyAdminURL, "http://caddy:2019")

	view, err := svc.All(context.Background())
	if err != nil {
		t.Fatalf("读取设置失败: %v", err)
	}
	if view[model.SettingKeyLoginMaxFails] != "5" {
		t.Fatalf("默认值应兜底: %v", view)
	}
	if view[model.SettingKeyAdminURL] != "http://caddy:2019" {
		t.Fatalf("settings 表应覆盖默认: %v", view)
	}
}
