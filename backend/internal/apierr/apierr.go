// Package apierr 定义稳定的业务错误码与统一错误响应。
// handler 与 middleware 共用;message 按 Accept-Language 翻译,detail 保留上游原文。
package apierr

import (
	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/i18n"
)

// 业务错误码:机器可读、稳定,前端可据此做精细处理(FR-603)。
const (
	CodeInvalidCredentials         = "AUTH_INVALID_CREDENTIALS"
	CodeAccountLocked              = "AUTH_ACCOUNT_LOCKED"
	CodeUnauthorized               = "AUTH_UNAUTHORIZED"
	CodeBadRequest                 = "BAD_REQUEST"
	CodeForbidden                  = "FORBIDDEN"
	CodeInternal                   = "INTERNAL_ERROR"
	CodeSiteNotFound               = "SITE_NOT_FOUND"
	CodeSiteDomainConflict         = "SITE_DOMAIN_CONFLICT"
	CodeSiteDeployFailed           = "SITE_DEPLOY_FAILED"
	CodeConfigDeployFailed         = "CONFIG_DEPLOY_FAILED"
	CodeConfigVersionNotApplicable = "CONFIG_VERSION_NOT_APPLICABLE"
	CodeCaddyUnreachable           = "CADDY_UNREACHABLE"
	CodeNotFound                   = "NOT_FOUND"
	CodeSettingInvalid             = "SETTING_INVALID"
)

// gin context 键:injectI18n 写入翻译包。
const ctxKeyI18n = "i18n_bundle"

// ErrorResponse 统一错误响应体(swaggo 文档引用)。
type ErrorResponse struct {
	Code    string `json:"code" example:"BAD_REQUEST"`
	Message string `json:"message" example:"请求参数不正确"`
	Detail  any    `json:"detail"`
}

// InjectI18n 把翻译包放进 context,RespondError 由此按请求语言取文案。
func InjectI18n(bundle *i18n.Bundle) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ctxKeyI18n, bundle)
		c.Next()
	}
}

// RespondError 统一错误响应:message 按 Accept-Language 翻译(FR-603),
// detail 保留上游原文(Caddy 错误等)不翻译。
func RespondError(c *gin.Context, status int, code, msgID string, msgData, detail any) {
	bundle := c.MustGet(ctxKeyI18n).(*i18n.Bundle)
	message := bundle.Localize(c.GetString(middleware.CtxKeyLang), msgID, i18n.FallbackText[msgID], msgData)
	c.JSON(status, ErrorResponse{Code: code, Message: message, Detail: detail})
}
