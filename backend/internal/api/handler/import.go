// Package handler — Caddyfile 导入接口(FR-705)。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// ImportHandler 导入接口。
type ImportHandler struct {
	repo service.ImportRepo
}

// NewImport 构造导入 handler。
func NewImport(repo service.ImportRepo) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// ImportBody 导入请求体(Caddyfile 全文)。
type ImportBody struct {
	Content string `json:"content" example:"app.example.com {\\n\\treverse_proxy 127.0.0.1:8080\\n}"`
}

// Preview godoc
// @Summary 导入预览
// @Description 解析 Caddyfile,返回可识别站点映射与未识别片段,不落库(FR-705)
// @Tags import
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.ImportBody true "Caddyfile 全文"
// @Success 200 {object} service.ImportPreview
// @Failure 400 {object} apierr.ErrorResponse "解析失败"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/import/preview [post]
func (h *ImportHandler) Preview() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body ImportBody
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		preview, err := service.PreviewImport(c.Request.Context(), body.Content)
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
				i18n.MsgBadRequest, map[string]any{"reason": err.Error()}, err.Error())
			return
		}
		c.JSON(http.StatusOK, preview)
	}
}

// ApplyResultResponse 导入结果响应。
type ApplyResultResponse struct {
	Result service.ImportApplyResult `json:"result"`
}

// Apply godoc
// @Summary 确认导入
// @Description 重新解析并落库为停用站点;域名冲突跳过;不自动下发(FR-705)
// @Tags import
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.ImportBody true "Caddyfile 全文"
// @Success 200 {object} handler.ApplyResultResponse
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/import/apply [post]
func (h *ImportHandler) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body ImportBody
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		preview, err := service.PreviewImport(c.Request.Context(), body.Content)
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
				i18n.MsgBadRequest, map[string]any{"reason": err.Error()}, err.Error())
			return
		}
		result, err := service.ImportApply(c.Request.Context(), h.repo, preview)
		if err != nil {
			respondConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, ApplyResultResponse{Result: *result})
	}
}
