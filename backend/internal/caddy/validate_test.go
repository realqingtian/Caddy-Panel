package caddy

import (
	"context"
	"strings"
	"testing"
)

// 离线适配为纯进程内能力(不触网),直接单测;真实 /load 链路归 integration 测试。
func TestValidate_OfflineAdaptsValidCaddyfile(t *testing.T) {
	v := NewValidator(nil)

	out, err := v.Validate(context.Background(), "localhost {\n\trespond \"ok\"\n}\n")
	if err != nil {
		t.Fatalf("合法 Caddyfile 应通过: %v", err)
	}
	if !strings.Contains(string(out), "\"apps\"") {
		t.Fatalf("适配产物应为 Caddy JSON: %s", out)
	}
}

func TestValidate_RejectsUnknownDirective(t *testing.T) {
	v := NewValidator(nil)

	_, err := v.Validate(context.Background(), "localhost {\n\ttotally_unknown_directive foo\n}\n")
	if err == nil {
		t.Fatal("未知指令应校验失败")
	}
	if !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("错误应包含定位信息: %v", err)
	}
}
