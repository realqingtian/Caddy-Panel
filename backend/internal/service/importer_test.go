package service

import (
	"context"
	"strings"
	"testing"

	"caddy-panel/backend/internal/model"
)

const importFixture = `{
	admin localhost:2019
	collectors {
		timeout 10s
	}
}

http://app.example.com {
	log {
		output file /var/log/a.log
	}
	reverse_proxy 127.0.0.1:8080 127.0.0.1:8081 {
		lb_policy least_conn
		flush_interval -1
		health_uri /healthz
		health_interval 10s
	}
	basic_auth {
		alice $2a$10$hashhere
	}
	tls internal
}

static.example.com {
	root * /var/www
	file_server
}

old.example.com {
	redir https://new.example.com permanent
}
`

func TestImportPreview_MapsRecognizedDirectives(t *testing.T) {
	preview, err := PreviewImport(context.Background(), importFixture)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if preview.GlobalExtra == "" || !containsStr(preview.GlobalExtra, "collectors") {
		t.Fatalf("未识别全局指令应落 global_extra: %q", preview.GlobalExtra)
	}
	if len(preview.Sites) != 3 {
		t.Fatalf("站点数不符: %d", len(preview.Sites))
	}

	proxy := preview.Sites[0]
	if proxy.Type != "proxy" || proxy.Upstreams != "127.0.0.1:8080, 127.0.0.1:8081" {
		t.Fatalf("proxy 映射不符: %+v", proxy)
	}
	if !proxy.Websocket || proxy.LBPolicy != "least_conn" {
		t.Fatalf("反代选项映射不符: ws=%v lb=%s", proxy.Websocket, proxy.LBPolicy)
	}
	if !proxy.HealthCheckEnabled || proxy.HealthCheckPath != "/healthz" {
		t.Fatalf("健康检查映射不符: %+v", proxy)
	}
	if proxy.HTTPSRedirect {
		t.Fatal("http:// 前缀地址应关闭强制跳转")
	}
	if !containsStr(proxy.BasicAuthUsers, "alice") || !containsStr(proxy.BasicAuthUsers, "$2a$10$hashhere") {
		t.Fatalf("basicauth 哈希应原样保留: %q", proxy.BasicAuthUsers)
	}
	// tls 指令:警告 + raw_extra。
	if len(proxy.Warnings) == 0 || !containsStr(proxy.RawExtra, "tls") {
		t.Fatalf("tls 应产生警告与片段: %+v", proxy)
	}

	staticSite := preview.Sites[1]
	if staticSite.Type != "static" || staticSite.RootPath != "/var/www" {
		t.Fatalf("static 映射不符: %+v", staticSite)
	}
	if !staticSite.HTTPSRedirect {
		t.Fatal("无 http:// 前缀应默认强制跳转")
	}

	redirect := preview.Sites[2]
	if redirect.Type != "redirect" || redirect.RedirectTo != "https://new.example.com" || !redirect.RedirectPermanent {
		t.Fatalf("redirect 映射不符: %+v", redirect)
	}
}

func TestImportApply_SkipsDomainConflicts(t *testing.T) {
	repo := newFakeSiteRepo()
	if err := repo.CreateSite(context.Background(), &model.Site{Name: "existing", Type: "static", Domains: "app.example.com"}); err != nil {
		t.Fatalf("准备冲突站点: %v", err)
	}

	preview, err := PreviewImport(context.Background(), importFixture)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	result, err := ImportApply(context.Background(), repo, preview)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(result.Created) != 2 || len(result.Skipped) != 1 {
		t.Fatalf("应创建 2 跳过 1: %+v", result)
	}
	// 导入站点一律停用。
	sites, _ := repo.ListSites(context.Background())
	for _, site := range sites {
		if site.Name == "static.example.com" && site.Enabled {
			t.Fatal("导入站点应保持停用")
		}
	}
}

func containsStr(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestBackupExportAndRestore(t *testing.T) {
	repo := newFakeSiteRepo()
	svc := NewBackup(repo, NewSetting(repo, map[string]string{model.SettingKeyLoginMaxFails: "5"}))
	ctx := context.Background()

	if err := repo.CreateSite(ctx, &model.Site{Name: "app", Type: "proxy", Domains: "a.com", Upstreams: "127.0.0.1:80"}); err != nil {
		t.Fatalf("准备站点: %v", err)
	}
	repo.recordVersionForTest("rendered", model.VersionStatusApplied, "[]")
	_ = repo.SetSetting(ctx, model.SettingKeyLoginMaxFails, "7")

	file, err := svc.Export(ctx)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if file.Version != backupFormatVersion || len(file.Sites) != 1 || file.Settings[model.SettingKeyLoginMaxFails] != "7" {
		t.Fatalf("备份内容不符: %+v", file.Settings)
	}

	// 清空后恢复。
	_ = repo.ReplaceSites(ctx, nil)
	_ = repo.ReplaceConfigVersions(ctx, nil)
	result, err := svc.Restore(ctx, file)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if result.Sites != 1 || result.Versions != 1 {
		t.Fatalf("恢复计数不符: %+v", result)
	}
	if v, _, _ := repo.GetSetting(ctx, model.SettingKeyLoginMaxFails); v != "7" {
		t.Fatalf("设置应恢复为 7,实际 %q", v)
	}

	// 版本不兼容拒绝。
	bad := *file
	bad.Version = 99
	if _, err := svc.Restore(ctx, &bad); err == nil {
		t.Fatal("未知版本应拒绝")
	}
}
