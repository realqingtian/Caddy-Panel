// Package i18n 提供后端用户可见消息的中/英翻译(FR-603)。
// 语言随请求 Accept-Language 头选择,默认 zh-CN;detail 中的上游原文不翻译。
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localeFS embed.FS

// 支持的语言标识;语言包文件名与之一致。
const (
	LangZh = "zh-CN"
	LangEn = "en-US"
)

// Bundle 持有 go-i18n 消息包,并发安全,启动时构建一次。
type Bundle struct {
	bundle *i18n.Bundle
}

// New 构建消息包:zh-CN 为默认语言(兜底),en-US 按需加载。
func New() (*Bundle, error) {
	b := i18n.NewBundle(language.Chinese)
	b.RegisterUnmarshalFunc("json", json.Unmarshal)
	for _, path := range []string{"locales/zh-CN.json", "locales/en-US.json"} {
		if _, err := b.LoadMessageFileFS(localeFS, path); err != nil {
			return nil, fmt.Errorf("加载语言包 %s: %w", path, err)
		}
	}
	return &Bundle{bundle: b}, nil
}

// NormalizeLang 从 Accept-Language 头解析支持的语言;
// M1 仅中/英,包含 en 视为英文,其余一律中文。
func NormalizeLang(acceptLang string) string {
	if strings.Contains(strings.ToLower(acceptLang), "en") {
		return LangEn
	}
	return LangZh
}

// Localize 翻译消息;未命中 ID 时回退到 fallback 文案,保证调用方总能拿到字符串。
// data 为模板变量(可为 nil)。
func (b *Bundle) Localize(lang, messageID, fallback string, data any) string {
	localizer := i18n.NewLocalizer(b.bundle, lang)
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: data,
		DefaultMessage: &i18n.Message{
			ID:    messageID,
			Other: fallback,
		},
	})
	if err != nil {
		return fallback
	}
	return msg
}
