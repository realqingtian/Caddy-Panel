package service

import "testing"

// TestBuildWebhookPayload_Golden:六渠道 payload 格式锁定(I6 设计 §1)。
func TestBuildWebhookPayload_Golden(t *testing.T) {
	const text = "[Caddy Panel] 证书即将到期"
	const raw = `{"event":"certificate_expiry"}`
	cases := []struct {
		typ  WebhookType
		want string
	}{
		{WebhookFeishu, `{"msg_type":"text","content":{"text":"[Caddy Panel] 证书即将到期"}}`},
		{WebhookDingTalk, `{"msgtype":"text","text":{"content":"[Caddy Panel] 证书即将到期"}}`},
		{WebhookWeCom, `{"msgtype":"text","text":{"content":"[Caddy Panel] 证书即将到期"}}`},
		{WebhookSlack, `{"text":"[Caddy Panel] 证书即将到期"}`},
		{WebhookDiscord, `{"content":"[Caddy Panel] 证书即将到期"}`},
		{WebhookJSON, raw}, // 通用 JSON 原样透传(向后兼容)
	}
	for _, c := range cases {
		if got := BuildWebhookPayload(c.typ, text, raw); got != c.want {
			t.Errorf("%s payload 不符:\n got %s\nwant %s", c.typ, got, c.want)
		}
	}
}

// TestResolveWebhookType:URL 自动识别与手动指定优先级。
func TestResolveWebhookType(t *testing.T) {
	urls := map[string]WebhookType{
		"https://open.feishu.cn/open-apis/bot/v2/hook/xxx":       WebhookFeishu,
		"https://oapi.dingtalk.com/robot/send?access_token=x":    WebhookDingTalk,
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=x": WebhookWeCom,
		"https://hooks.slack.com/services/a/b/c":                 WebhookSlack,
		"https://discord.com/api/webhooks/1/xxx":                 WebhookDiscord,
		"https://self.example.com/hook":                          WebhookJSON,
	}
	for url, want := range urls {
		if got := ResolveWebhookType("auto", url); got != want {
			t.Errorf("auto(%s) = %s, 期望 %s", url, got, want)
		}
	}
	// 手动指定优先于 URL 特征。
	if got := ResolveWebhookType("json", "https://open.feishu.cn/open-apis/bot/v2/hook/x"); got != WebhookJSON {
		t.Errorf("手动指定应优先,实际 %s", got)
	}
}

// TestIsPlatformSuccess:平台业务码判定(仅 HTTP 2xx 不够)。
func TestIsPlatformSuccess(t *testing.T) {
	cases := []struct {
		typ  WebhookType
		body string
		ok   bool
	}{
		{WebhookFeishu, `{"code":0,"msg":"success"}`, true},
		{WebhookFeishu, `{"code":19021,"msg":"keyword not found"}`, false},
		{WebhookDingTalk, `{"errcode":0,"errmsg":"ok"}`, true},
		{WebhookDingTalk, `{"errcode":310000,"errmsg":"sign error"}`, false},
		{WebhookWeCom, `{"errcode":0}`, true},
		{WebhookSlack, `{"ok":true}`, true},
		{WebhookDiscord, ``, true}, // Discord 空 body 即成功
		{WebhookJSON, `anything`, true},
	}
	for _, c := range cases {
		if ok, _ := IsPlatformSuccess(c.typ, c.body); ok != c.ok {
			t.Errorf("%s body %s: 期望 ok=%v", c.typ, c.body, c.ok)
		}
	}
}
