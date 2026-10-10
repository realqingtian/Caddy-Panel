// Package handler — Dashboard 状态聚合接口(FR-201/202)。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/service"
)

// DashboardHandler 状态聚合接口。
type DashboardHandler struct {
	dashboard *service.Dashboard
}

// NewDashboard 构造 dashboard handler。
func NewDashboard(dashboard *service.Dashboard) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard}
}

// Status godoc
// @Summary 运行状态总览
// @Description Caddy 存活/版本、Panel 版本、站点计数、最近下发与漂移检测(FR-201/202)
// @Tags dashboard
// @Produce json
// @Security BearerAuth
// @Success 200 {object} service.StatusSummary
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/status [get]
func (h *DashboardHandler) Status() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, h.dashboard.Summary(c.Request.Context()))
	}
}
