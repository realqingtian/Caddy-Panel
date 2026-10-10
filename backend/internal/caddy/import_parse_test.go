package caddy

import (
	"strings"
	"testing"
)

func TestParseCaddyfile_ProxySite(t *testing.T) {
	content := `{
	admin localhost:2019
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
		health_status 200
	}
	header X-Foo bar
	basic_auth {
		alice $2a$10$hashhere
	}
}
`
	parsed, err := ParseCaddyfile(content)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(parsed.Global) != 1 || parsed.Global[0].Name != "admin" {
		t.Fatalf("全局块解析不符: %+v", parsed.Global)
	}
	if len(parsed.Sites) != 1 {
		t.Fatalf("站点数不符: %d", len(parsed.Sites))
	}
	site := parsed.Sites[0]
	if len(site.Addresses) != 1 || site.Addresses[0] != "http://app.example.com" {
		t.Fatalf("地址不符: %v", site.Addresses)
	}

	byName := map[string]ParsedDirective{}
	for _, dir := range site.Directives {
		byName[dir.Name] = dir
	}
	proxy := byName["reverse_proxy"]
	if len(proxy.Args) != 2 || proxy.Args[0] != "127.0.0.1:8080" {
		t.Fatalf("上游解析不符: %+v", proxy.Args)
	}
	if len(proxy.Children) != 5 {
		t.Fatalf("reverse_proxy 子指令数不符: %d(%+v)", len(proxy.Children), proxy.Children)
	}
	if proxy.Children[0].Name != "lb_policy" || proxy.Children[0].Args[0] != "least_conn" {
		t.Fatalf("lb_policy 解析不符: %+v", proxy.Children[0])
	}
	if proxy.Children[4].Name != "health_status" || proxy.Children[4].Args[0] != "200" {
		t.Fatalf("health_status 解析不符: %+v", proxy.Children[4])
	}
	auth := byName["basic_auth"]
	if len(auth.Children) != 1 || auth.Children[0].Name != "alice" {
		t.Fatalf("basic_auth 解析不符: %+v", auth.Children)
	}
}

func TestParseCaddyfile_SyntaxError(t *testing.T) {
	if _, err := ParseCaddyfile("site { unclosed"); err == nil {
		t.Fatal("语法错误应返回错误")
	}
}

func TestRebuildDirective_QuotesSpaces(t *testing.T) {
	dir := ParsedDirective{Name: "header", Args: []string{"X-Foo", "two words"}}
	rebuilt := RebuildDirective(dir, 1)
	if !strings.Contains(rebuilt, `"two words"`) {
		t.Fatalf("含空格参数应加引号: %q", rebuilt)
	}
}
