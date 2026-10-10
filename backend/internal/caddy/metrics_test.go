package caddy

import (
	"strings"
	"testing"
)

func TestParsePrometheus(t *testing.T) {
	text := `# HELP caddy_http_requests_total HTTP requests
# TYPE caddy_http_requests_total counter
caddy_http_requests_total{host="a.com",handler="http.handlers.reverse_proxy"} 42
caddy_http_requests_total{host="b.com"} 7
other_metric 999
malformed-line
`
	samples := ParsePrometheus(text, "caddy_http_")
	if len(samples) != 2 {
		t.Fatalf("应过滤到 2 条 caddy_http 样本,实际 %d", len(samples))
	}
	if samples[0].Name != "caddy_http_requests_total" || samples[0].Value != 42 {
		t.Fatalf("样本 0 不符: %+v", samples[0])
	}
	if samples[0].Labels["host"] != "a.com" || samples[0].Labels["handler"] != "http.handlers.reverse_proxy" {
		t.Fatalf("标签解析不符: %+v", samples[0].Labels)
	}
	if samples[1].Value != 7 || samples[1].Labels["host"] != "b.com" {
		t.Fatalf("样本 1 不符: %+v", samples[1])
	}
}

func TestParsePrometheus_QuotedLabelComma(t *testing.T) {
	text := `caddy_http_x{path="/a,b"} 1
`
	samples := ParsePrometheus(text, "caddy_http_")
	if len(samples) != 1 || samples[0].Labels["path"] != "/a,b" {
		t.Fatalf("引号内逗号应保留: %+v", samples)
	}
}

func TestFormatSampleKey_OrdersLabels(t *testing.T) {
	name, labels := FormatSampleKey("m", map[string]string{"b": "2", "a": "1"})
	if name != "m" {
		t.Fatalf("name 不符: %q", name)
	}
	if !strings.Contains(labels, `"a": "1"`) || !strings.Contains(labels, `"b": "2"`) {
		t.Fatalf("标签应排序稳定: %q", labels)
	}
	if strings.Index(labels, `"a"`) > strings.Index(labels, `"b"`) {
		t.Fatal("a 应排在 b 前")
	}
}
