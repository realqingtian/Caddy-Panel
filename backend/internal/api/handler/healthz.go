// Package handler 存放 HTTP handler:仅做参数绑定与协议转换,业务逻辑一律在 service 层。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/version"
)

// HealthResponse /healthz 响应体。
type HealthResponse struct {
	Status  string `json:"status" example:"ok"`
	Version string `json:"version" example:"0.1.0-dev"`
}

// Healthz godoc
// @Summary 健康检查
// @Description 供容器探活与前端连通性检查,不要求鉴权
// @Tags system
// @Produce json
// @Success 200 {object} handler.HealthResponse
// @Router /healthz [get]
func Healthz() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthResponse{Status: "ok", Version: version.Version})
	}
}
