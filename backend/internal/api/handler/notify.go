// Package handler — 通知渠道辅助接口:SMTP 配置测试发送(FR-703,I6)。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// NotifyHandler 通知渠道接口。
type NotifyHandler struct {
	mailer *service.SMTPMailer
}

// NewNotify 构造通知渠道 handler。
func NewNotify(mailer *service.SMTPMailer) *NotifyHandler {
	return &NotifyHandler{mailer: mailer}
}

// SMTPTest godoc
// @Summary 发送 SMTP 测试邮件
// @Description 按当前 SMTP 配置向已配置的收件人发一封测试邮件,用于验证渠道可用(FR-703)
// @Tags settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SimpleOK
// @Failure 400 {object} apierr.ErrorResponse "SMTP 未配置完整"
// @Failure 401 {object} apierr.ErrorResponse
// @Failure 502 {object} apierr.ErrorResponse "发送失败(detail 含原因)"
// @Router /api/v1/settings/smtp/test [post]
func (h *NotifyHandler) SMTPTest() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok, err := h.mailer.Load(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		if !ok {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest,
				map[string]any{"reason": "SMTP 服务器/端口/用户名/收件人未配置完整"}, "smtp config incomplete")
			return
		}
		if err := h.mailer.Send(cfg, "[Caddy Panel] SMTP 配置测试邮件", "这是一封测试邮件:收到即说明 Caddy Panel 的 SMTP 通知渠道配置成功。"); err != nil {
			apierr.RespondError(c, http.StatusBadGateway, apierr.CodeCaddyUnreachable, i18n.MsgSMTPTestFailed, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
