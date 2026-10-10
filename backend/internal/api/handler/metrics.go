// Package handler — metrics 查询接口(FR-702)。
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// MetricsHandler 监控接口。
type MetricsHandler struct {
	collector *service.MetricsCollector
}

// NewMetrics 构造 metrics handler。
func NewMetrics(collector *service.MetricsCollector) *MetricsHandler {
	return &MetricsHandler{collector: collector}
}

// allowedWindows 支持的查询窗口。
var allowedWindows = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
}

// Summary godoc
// @Summary 指标时间序列
// @Description 返回指定节点窗口内样本的降采样序列(60 个点);node_id 缺省为 local(FR-702)
// @Tags metrics
// @Produce json
// @Security BearerAuth
// @Param window query string false "窗口" Enums(1h,6h,24h)
// @Param node_id query string false "节点标识" default(local)
// @Success 200 {object} service.MetricsSummary
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/metrics/summary [get]
func (h *MetricsHandler) Summary() gin.HandlerFunc {
	return func(c *gin.Context) {
		window, ok := allowedWindows[c.DefaultQuery("window", "1h")]
		if !ok {
			window = time.Hour
		}
		nodeID := c.DefaultQuery("node_id", model.NodeIDLocal)
		summary, err := h.collector.Summary(c.Request.Context(), nodeID, window)
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusOK, summary)
	}
}

// PrometheusConfig godoc
// @Summary 生成 Prometheus 抓取配置
// @Description 返回 scrape_configs YAML 示例文本(FR-702)
// @Tags metrics
// @Produce plain
// @Security BearerAuth
// @Param host query string false "Caddy 可被 Prometheus 访问的主机:端口" default(caddy:80)
// @Success 200 {string} string
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/metrics/prometheus-config [get]
func (h *MetricsHandler) PrometheusConfig() gin.HandlerFunc {
	return func(c *gin.Context) {
		host := c.DefaultQuery("host", "caddy:80")
		c.String(http.StatusOK, service.PrometheusConfig(host))
	}
}
