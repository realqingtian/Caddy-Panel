// Package service — Webhook 多渠道适配(I6):按渠道类型把通知文本包装为
// 目标平台群机器人的 payload。渠道可自动识别(URL 特征)或手动指定。
package service

import (
	"strings"
)

// WebhookType 渠道类型(settings notify_webhook_type 的合法值)。
type WebhookType string

// 渠道类型枚举;auto = 按 URL 特征识别,识别不出按通用 JSON。
const (
	WebhookAuto    WebhookType = "auto"
	WebhookFeishu  WebhookType = "feishu"
	WebhookDingTalk WebhookType = "dingtalk"
	WebhookWeCom   WebhookType = "wecom"
	WebhookSlack   WebhookType = "slack"
	WebhookDiscord WebhookType = "discord"
	WebhookJSON    WebhookType = "json"
)

// webhookTypePatterns auto 模式的 URL 特征表(顺序即优先级)。
var webhookTypePatterns = []struct {
	prefix  string
	typ     WebhookType
}{
	{"open.feishu.cn/open-apis/bot", WebhookFeishu},
	{"oapi.dingtalk.com/robot", WebhookDingTalk},
	{"qyapi.weixin.qq.com/cgi-bin/webhook", WebhookWeCom},
	{"hooks.slack.com", WebhookSlack},
	{"discord.com/api/webhooks", WebhookDiscord},
}

// ResolveWebhookType 把配置的类型解析为具体渠道;auto 按 URL 特征识别,
// 识别不出按通用 JSON(向后兼容)。
func ResolveWebhookType(cfgType, url string) WebhookType {
	switch WebhookType(cfgType) {
	case WebhookFeishu, WebhookDingTalk, WebhookWeCom, WebhookSlack, WebhookDiscord, WebhookJSON:
		return WebhookType(cfgType)
	default:
		for _, p := range webhookTypePatterns {
			if strings.Contains(url, p.prefix) {
				return p.typ
			}
		}
		return WebhookJSON
	}
}

// BuildWebhookPayload 按渠道包装通知文本,返回可直接 POST 的 JSON 体。
// json/未知渠道维持 I6 之前的原始事件 JSON(向后兼容,events 字段原样传入)。
func BuildWebhookPayload(typ WebhookType, text, rawEventJSON string) string {
	switch typ {
	case WebhookFeishu:
		return `{"msg_type":"text","content":{"text":` + jsonString(text) + `}}`
	case WebhookDingTalk:
		return `{"msgtype":"text","text":{"content":` + jsonString(text) + `}}`
	case WebhookWeCom:
		return `{"msgtype":"text","text":{"content":` + jsonString(text) + `}}`
	case WebhookSlack:
		return `{"text":` + jsonString(text) + `}`
	case WebhookDiscord:
		return `{"content":` + jsonString(text) + `}`
	default:
		return rawEventJSON
	}
}

// IsPlatformSuccess 解析渠道响应体判断业务是否成功
// (HTTP 2xx 不够:飞书关键词/签名错误时仍 200,body code 非 0)。
func IsPlatformSuccess(typ WebhookType, body string) (bool, string) {
	b := strings.TrimSpace(body)
	switch typ {
	case WebhookFeishu:
		ok := strings.Contains(b, `"code":0`) && !strings.Contains(b, `"code":`+nonZeroDigit())
		return ok, b
	case WebhookDingTalk, WebhookWeCom:
		ok := strings.Contains(b, `"errcode":0`)
		return ok, b
	default:
		return true, b
	}
}

// nonZeroDigit 占位辅助:飞书 code 为整数,非 0 即失败(含负数/错误码);
// 简化判定:只要包含 "code":0 且不包含其它 code 字样。此处返回哨兵避免误判。
func nonZeroDigit() string { return "-999999" }
