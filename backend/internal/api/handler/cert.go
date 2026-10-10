// Package handler — 站点证书状态接口(FR-308)。
package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// copyTo io.Reader → bytes.Buffer。
func copyTo(dst *bytes.Buffer, src interface{ Read([]byte) (int, error) }) (int64, error) {
	return io.Copy(dst, src)
}

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

// UploadCertificate godoc
// @Summary 上传站点证书
// @Description multipart 上传 cert/key(PEM);校验配对后保存并写入站点,下次下发生效(FR-703)
// @Tags sites
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param id path int true "站点 ID"
// @Param cert formData file true "证书 PEM"
// @Param key formData file true "私钥 PEM"
// @Success 200 {object} model.Site
// @Failure 400 {object} apierr.ErrorResponse "证书/私钥非法或不匹配"
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/sites/{id}/certificate/upload [post]
func (h *CertHandler) UploadCertificate() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		certHeader, err := c.FormFile("cert")
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest,
				map[string]any{"reason": "缺少 cert 文件"}, err.Error())
			return
		}
		keyHeader, err := c.FormFile("key")
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest,
				map[string]any{"reason": "缺少 key 文件"}, err.Error())
			return
		}
		certPEM, err := certHeader.Open()
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		defer func() { _ = certPEM.Close() }()
		keyPEM, err := keyHeader.Open()
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		defer func() { _ = keyPEM.Close() }()

		certBuf, keyBuf := new(bytes.Buffer), new(bytes.Buffer)
		if _, err := copyTo(certBuf, certPEM); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		if _, err := copyTo(keyBuf, keyPEM); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}

		site, err := h.cert.UploadCertificate(c.Request.Context(), id, certBuf.Bytes(), keyBuf.Bytes())
		if err != nil {
			var invalid *service.InvalidInputError
			if errors.As(err, &invalid) {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
					i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
				return
			}
			respondSiteError(c, err)
			return
		}
		c.Set(middleware.CtxKeyAuditTarget, site.Name)
		c.JSON(http.StatusOK, site)
	}
}
