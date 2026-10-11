// Package handler — 配置中心接口:版本历史、回滚、直编、当前配置(FR-40x)。
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// ConfigHandler 配置中心接口。
type ConfigHandler struct {
	config *service.Config
}

// NewConfig 构造配置中心 handler。
func NewConfig(config *service.Config) *ConfigHandler {
	return &ConfigHandler{config: config}
}

// VersionListResponse 版本历史响应。
type VersionListResponse struct {
	Versions []model.ConfigVersion `json:"versions"`
}

// ListVersions godoc
// @Summary 配置版本历史
// @Description 返回最近 100 条下发记录,新→旧(FR-402)
// @Tags config
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.VersionListResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/config/versions [get]
func (h *ConfigHandler) ListVersions() gin.HandlerFunc {
	return func(c *gin.Context) {
		versions, err := h.config.ListVersions(c.Request.Context())
		if err != nil {
			respondConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, VersionListResponse{Versions: versions})
	}
}

// GetVersion godoc
// @Summary 版本详情
// @Description 含 Caddyfile 全文与站点快照,供前端 diff(FR-402)
// @Tags config
// @Produce json
// @Security BearerAuth
// @Param id path int true "版本 ID"
// @Success 200 {object} model.ConfigVersion
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/config/versions/{id} [get]
func (h *ConfigHandler) GetVersion() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		version, err := h.config.GetVersion(c.Request.Context(), id)
		if err != nil {
			respondConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, version)
	}
}

// Rollback godoc
// @Summary 回滚到指定版本
// @Description 恢复该版本渲染产物与站点快照并生成新版本记录(FR-403)
// @Tags config
// @Produce json
// @Security BearerAuth
// @Param id path int true "版本 ID"
// @Success 200 {object} model.ConfigVersion
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 409 {object} apierr.ErrorResponse "目标不是成功版本"
// @Failure 422 {object} apierr.ErrorResponse "下发失败(detail 含 Caddy 原文)"
// @Router /api/v1/config/versions/{id}/rollback [post]
func (h *ConfigHandler) Rollback() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		version, err := h.config.Rollback(c.Request.Context(), id)
		if err != nil {
			respondConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, version)
	}
}

// SaveCaddyfileBody Caddyfile 直编请求体。
type SaveCaddyfileBody struct {
	Content string `json:"content" example:"localhost { respond \"ok\" }"`
}

// SaveCaddyfile godoc
// @Summary 直接编辑 Caddyfile
// @Description 校验通过才下发落盘并生成版本记录;失败不生效(FR-404)
// @Tags config
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.SaveCaddyfileBody true "Caddyfile 全文"
// @Success 200 {object} model.ConfigVersion
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 422 {object} apierr.ErrorResponse "校验或下发失败(detail 含 Caddy 原文)"
// @Router /api/v1/config/caddyfile [post]
func (h *ConfigHandler) SaveCaddyfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body SaveCaddyfileBody
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		version, err := h.config.SaveCaddyfile(c.Request.Context(), body.Content)
		if err != nil {
			respondConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, version)
	}
}

// CurrentCaddyfileResponse 当前渲染产物响应。
type CurrentCaddyfileResponse struct {
	Content string `json:"content"`
}

// CurrentCaddyfile godoc
// @Summary 当前渲染产物
// @Description 返回磁盘上的 Caddyfile 全文(FR-401)
// @Tags config
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.CurrentCaddyfileResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/config/caddyfile [get]
func (h *ConfigHandler) CurrentCaddyfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		content, err := h.config.CurrentCaddyfile(c.Request.Context())
		if err != nil {
			respondConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, CurrentCaddyfileResponse{Content: content})
	}
}

// CurrentJSON godoc
// @Summary Caddy 当前生效配置
// @Description GET /config 透传,只读展示(FR-401)
// @Tags config
// @Produce json
// @Security BearerAuth
// @Success 200 {string} string "Caddy 配置 JSON"
// @Failure 401 {object} apierr.ErrorResponse
// @Failure 502 {object} apierr.ErrorResponse "Caddy 不可达"
// @Router /api/v1/config/current [get]
func (h *ConfigHandler) CurrentJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		config, err := h.config.CurrentJSON(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusBadGateway, apierr.CodeCaddyUnreachable,
				i18n.MsgCaddyUnreachable, nil, err.Error())
			return
		}
		// 透传 Caddy 原始 JSON。
		c.Data(http.StatusOK, "application/json", config)
	}
}

// UpdateVersionNotesBody 版本备注请求体。
type UpdateVersionNotesBody struct {
	Notes string `json:"notes" example:"接入示例站点前的可用版本"`
}

// UpdateVersionNotes godoc
// @Summary 编辑版本备注
// @Description 设置/清空版本备注;空字符串即清空(logux-design §8)
// @Tags config
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "版本 ID"
// @Param body body handler.UpdateVersionNotesBody true "备注内容"
// @Success 200 {object} model.ConfigVersion
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/config/versions/{id}/notes [patch]
func (h *ConfigHandler) UpdateVersionNotes() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var body UpdateVersionNotesBody
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		version, err := h.config.UpdateVersionNotes(c.Request.Context(), id, body.Notes)
		if err != nil {
			respondConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, version)
	}
}

// respondConfigError 配置中心业务错误 → 统一错误响应。
func respondConfigError(c *gin.Context, err error) {
	var deploy *service.DeployError
	var invalid *service.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
			i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
	case errors.As(err, &deploy):
		apierr.RespondError(c, http.StatusUnprocessableEntity, apierr.CodeConfigDeployFailed,
			i18n.MsgConfigDeployFailed, nil, deploy.Error())
	case errors.Is(err, service.ErrVersionNotFound):
		apierr.RespondError(c, http.StatusNotFound, apierr.CodeNotFound, i18n.MsgNotFound, nil, nil)
	case errors.Is(err, service.ErrVersionNotApplicable):
		apierr.RespondError(c, http.StatusConflict, apierr.CodeConfigVersionNotApplicable,
			i18n.MsgVersionNotApplicable, nil, nil)
	default:
		apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
	}
}
