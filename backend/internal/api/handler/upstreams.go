// Package handler — 上游状态接口(FR-701)。
package handler

import (
	"encoding/json"
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
// @Description 归一化 Caddy `GET /reverse_proxy/upstreams`:每项含 dial(上游地址)与请求/失败计数
// @Tags upstreams
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any `{"upstreams":[{"dial":"app:8080","num_requests":0}]}`
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
		normalized, err := normalizeUpstreams(upstreams)
		if err != nil {
			apierr.RespondError(c, http.StatusBadGateway, apierr.CodeCaddyUnreachable, i18n.MsgCaddyUnreachable, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"upstreams": normalized})
	}
}

// normalizeUpstreams 把 Caddy 原始 upstreams JSON 归一化为前端契约:
// 每项统一含 dial 字段(Caddy 实测字段为 address,兼容 dial),其余计数原样保留。
func normalizeUpstreams(raw json.RawMessage) ([]map[string]any, error) {
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := make(map[string]any, len(item))
		for k, v := range item {
			entry[k] = v
		}
		if _, ok := entry["dial"]; !ok {
			if addr, ok := entry["address"]; ok {
				entry["dial"] = addr
			}
		}
		if _, ok := entry["dial"]; ok {
			out = append(out, entry)
		}
	}
	return out, nil
}
