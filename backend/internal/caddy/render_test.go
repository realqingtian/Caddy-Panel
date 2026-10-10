package caddy

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"caddy-panel/backend/internal/model"
)

// golden 文件测试:-update 重写期望产物;正常跑逐字节对比。
var update = flag.Bool("update", false, "重写 golden 文件")

// renderFixture 覆盖三类站点与关键开关的固定输入。
func renderFixture() RenderInput {
	return RenderInput{
		Sites: []model.Site{
			{
				Name: "app", Type: model.SiteTypeProxy,
				Domains:        "app.example.com, www.example.com",
				Upstreams:      "127.0.0.1:8080, 127.0.0.1:8081",
				LBPolicy:       model.LBPolicyRoundRobin,
				Websocket:      true,
				HeadersEnabled: true,
				CustomHeaders:  `[{"key":"X-Frame-Options","value":"DENY"},{"key":"X-Served-By","value":"caddy \"panel\""}]`,
				HTTPSRedirect:  true,
				Enabled:        true,

				HealthCheckEnabled:  true,
				HealthCheckPath:     "/healthz",
				HealthCheckInterval: "10s",
				HealthCheckExpected: 200,

				IPBlocklist:    "192.168.1.1, 10.0.0.0/8",
				BasicAuthUsers: `[{"username":"alice","password_hash":"$2a$10$abcdefghij"}]`,
			},
			{
				Name: "api", Type: model.SiteTypeProxy,
				Domains:       "api.example.com",
				Upstreams:     "10.0.0.1:80, 10.0.0.2:80",
				LBPolicy:      model.LBPolicyLeastConn,
				HTTPSRedirect: true,
				Enabled:       true,
			},
			{
				Name: "web", Type: model.SiteTypeStatic,
				Domains:       "static.example.com",
				RootPath:      "/var/www/static",
				HTTPSRedirect: true,
				Enabled:       true,
				IPAllowlist:   "203.0.113.0/24",
			},
			{
				Name: "old", Type: model.SiteTypeRedirect,
				Domains:           "old.example.com",
				RedirectTo:        "https://new.example.com",
				RedirectPermanent: true,
				HTTPSRedirect:     false, // 关闭跳转:地址显式带 http:// 前缀
				Enabled:           true,
			},
			{
				Name: "disabled", Type: model.SiteTypeStatic,
				Domains: "off.example.com", RootPath: "/var/www/off",
				Enabled: false, // 停用站点不得出现在产物中
			},
		},
		Global: GlobalOptions{
			AdminBind:      "localhost:2019",
			ErrorLogPath:   "/data/logs/error.log",
			AccessLogPath:  "/data/logs/access.log",
			MetricsEnabled: true,
			GlobalExtra:    "\tservers {\n\t\tprotocols h1 h2 h3\n\t}",
		},
	}
}

func TestRender_Golden(t *testing.T) {
	got, err := NewRenderer().Render(renderFixture())
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}

	golden := filepath.Join("testdata", "golden.caddyfile")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("重写 golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("读取 golden(首次请 go test -run TestRender_Golden -update): %v", err)
	}
	if string(want) != got {
		t.Fatalf("渲染产物与 golden 不一致。\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func TestRender_EmptySites_StillRendersGlobal(t *testing.T) {
	got, err := NewRenderer().Render(RenderInput{Global: GlobalOptions{
		AdminBind: "localhost:2019", ErrorLogPath: "/data/logs/error.log",
	}})
	if err != nil {
		t.Fatalf("空站点渲染失败: %v", err)
	}
	if !strings.Contains(got, "admin localhost:2019") {
		t.Fatalf("空站点也应渲染全局块: %s", got)
	}
	if strings.Contains(got, "example.com") {
		t.Fatal("不应出现站点内容")
	}
}

// 渲染产物必须能被离线适配器接受:渲染与校验的契约保证。
func TestRender_OutputAdaptsOffline(t *testing.T) {
	got, err := NewRenderer().Render(renderFixture())
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	if _, err := NewValidator(nil).Validate(context.Background(), got); err != nil {
		t.Fatalf("渲染产物未通过离线校验(模板或转换层有语法问题): %v\n产物:\n%s", err, got)
	}
}
