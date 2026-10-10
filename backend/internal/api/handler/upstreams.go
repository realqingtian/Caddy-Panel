// Package handler — 上游状态接口(FR-701)。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/i18n"
)

// apierrRespond 上游接口的统一失败响应(Caddy 不可达/未配置)。
func apierrRespond(c *gin.Context) {
	apierr.RespondError(c, http.StatusBadGateway, apierr.CodeCaddyUnreachable, i18n.MsgCaddyUnreachable, nil, nil)
}

// UpstreamsHandler 反代上游状态接口。
type UpstreamsHandler struct {
	client caddy.Client
}

// NewUpstreams 构造上游状态 handler;client 为 nil(Caddy 未配置)时返回 502。
func NewUpstreams(client caddy.Client) *UpstreamsHandler {
	return &UpstreamsHandler{client: client}
}

// List godoc
// @Summary 反代上游实时状态
// @Description 透传 Caddy `GET /reverse_proxy/upstreams`:地址、活跃连接、请求/失败计数(FR-701)
// @Tags upstreams
// @Produce json
// @Security BearerAuth
// @Success 200 {string} string "Caddy upstreams JSON"
// @Failure 401 {object} apierr.ErrorResponse
// @Failure 502 {object} apierr.ErrorResponse "Caddy 不可达"
// @Router /api/v1/upstreams [get]
func (h *UpstreamsHandler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.client == nil {
			apierrRespond(c)
			return
		}
		upstreams, err := h.client.Upstreams(c.Request.Context())
		if err != nil {
			apierrRespond(c)
			return
		}
		c.Data(http.StatusOK, "application/json", upstreams)
	}
}
