// Package handler — 站点证书状态接口(FR-308)。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/service"
)

// CertHandler 证书状态接口。
type CertHandler struct {
	cert *service.CertService
}

// NewCert 构造证书 handler。
func NewCert(cert *service.CertService) *CertHandler {
	return &CertHandler{cert: cert}
}

// SiteCertificate godoc
// @Summary 站点证书状态
// @Description 只读返回各域名证书签发状态与到期时间;数据来自 Caddy 证书存储(FR-308)
// @Tags sites
// @Produce json
// @Security BearerAuth
// @Param id path int true "站点 ID"
// @Success 200 {object} service.SiteCertStatus
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/sites/{id}/certificate [get]
func (h *CertHandler) SiteCertificate() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		status, err := h.cert.ForSite(c.Request.Context(), id)
		if err != nil {
			respondSiteError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}
