// Package service — Webhook 多渠道适配(I6):按渠道类型把通知文本包装为
// 目标平台群机器人的 payload。渠道可自动识别(URL 特征)或手动指定。
package service

import (
	"encoding/json"
	"strings"
)

// WebhookType 渠道类型(settings notify_webhook_type 的合法值)。
type WebhookType string

// 渠道类型枚举;auto = 按 URL 特征识别,识别不出按通用 JSON。
const (
	WebhookAuto     WebhookType = "auto"
	WebhookFeishu   WebhookType = "feishu"
	WebhookDingTalk WebhookType = "dingtalk"
	WebhookWeCom    WebhookType = "wecom"
	WebhookSlack    WebhookType = "slack"
	WebhookDiscord  WebhookType = "discord"
	WebhookJSON     WebhookType = "json"
)

// webhookTypePatterns auto 模式的 URL 特征表。
var webhookTypePatterns = []struct {
	feature string
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
	}
	for _, p := range webhookTypePatterns {
		if strings.Contains(url, p.feature) {
			return p.typ
		}
	}
	return WebhookJSON
}

// BuildWebhookPayload 按渠道包装通知文本,返回可直接 POST 的 JSON 体。
// json/未知渠道维持 I6 之前的原始事件 JSON(rawEventJSON 原样,向后兼容)。
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

// IsPlatformSuccess 解析渠道响应体判断业务是否成功(HTTP 2xx 不够:
// 飞书关键词/签名错误时仍 200 但 body code 非 0;钉钉/企微为 errcode)。
func IsPlatformSuccess(typ WebhookType, body string) (bool, string) {
	b := strings.TrimSpace(body)
	switch typ {
	case WebhookFeishu:
		return platformCodeZero(b, "code")
	case WebhookDingTalk, WebhookWeCom:
		return platformCodeZero(b, "errcode")
	default:
		return true, b
	}
}

// platformCodeZero 判定 body JSON 中指定业务码字段是否为 0;非 JSON 响应视为失败。
func platformCodeZero(body, field string) (bool, string) {
	var m map[string]json.Number
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return false, body
	}
	code, ok := m[field]
	if !ok {
		return false, body
	}
	return code.String() == "0", body
}

// jsonString JSON 字符串转义(引号/换行等),用于手工拼装 payload。
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
