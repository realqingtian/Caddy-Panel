// Package handler — 通知渠道辅助接口:SMTP 配置测试发送(FR-703,I6)。
package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// NotifyHandler 通知渠道接口。
type NotifyHandler struct {
	mailer   *service.SMTPMailer
	settings service.SettingReader
	client   *http.Client
}

// NewNotify 构造通知渠道 handler。
func NewNotify(mailer *service.SMTPMailer, settings service.SettingReader) *NotifyHandler {
	return &NotifyHandler{mailer: mailer, settings: settings, client: &http.Client{Timeout: 10 * time.Second}}
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

// WebhookTest godoc
// @Summary 发送 Webhook 测试消息
// @Description 按当前 Webhook URL 与渠道类型真实 POST 一条测试消息,验证连通性(I6)
// @Tags settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SimpleOK
// @Failure 400 {object} apierr.ErrorResponse "Webhook URL 未配置"
// @Failure 401 {object} apierr.ErrorResponse
// @Failure 502 {object} apierr.ErrorResponse "发送失败或平台业务码非 0(detail 含返回体)"
// @Router /api/v1/settings/webhook/test [post]
func (h *NotifyHandler) WebhookTest() gin.HandlerFunc {
	return func(c *gin.Context) {
		url, _, err := h.settings.GetSetting(c.Request.Context(), model.SettingKeyNotifyWebhook)
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		if strings.TrimSpace(url) == "" {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest,
				map[string]any{"reason": "Webhook URL 未配置"}, "webhook url empty")
			return
		}
		cfgType, _, _ := h.settings.GetSetting(c.Request.Context(), model.SettingKeyNotifyWebhookType)
		typ := service.ResolveWebhookType(cfgType, url)
		payload := service.BuildWebhookPayload(typ, "[Caddy Panel] Webhook 渠道测试消息:收到即说明渠道配置成功。", `{}`)
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader([]byte(payload)))
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := h.client.Do(req)
		if err != nil {
			apierr.RespondError(c, http.StatusBadGateway, apierr.CodeCaddyUnreachable, i18n.MsgWebhookTestFailed, nil, err.Error())
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if resp.StatusCode >= 300 {
			apierr.RespondError(c, http.StatusBadGateway, apierr.CodeCaddyUnreachable, i18n.MsgWebhookTestFailed,
				nil, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
			return
		}
		if ok, detail := service.IsPlatformSuccess(typ, string(body)); !ok {
			apierr.RespondError(c, http.StatusBadGateway, apierr.CodeCaddyUnreachable, i18n.MsgWebhookTestFailed,
				nil, detail)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
