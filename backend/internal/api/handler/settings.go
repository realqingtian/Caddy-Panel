// Package handler — 系统设置接口(FR-601)。
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// SettingsHandler 系统设置接口。
type SettingsHandler struct {
	settings *service.SettingService
}

// NewSettings 构造设置 handler。
func NewSettings(settings *service.SettingService) *SettingsHandler {
	return &SettingsHandler{settings: settings}
}

// SettingsResponse 设置读取响应(键 → 生效值)。
type SettingsResponse struct {
	Settings map[string]string `json:"settings"`
}

// UpdateSettingsBody 设置更新请求体(键 → 新值)。
type UpdateSettingsBody struct {
	Settings map[string]string `json:"settings"`
}

// Get godoc
// @Summary 读取系统设置
// @Description 返回全部可编辑设置项的生效值(env 默认兜底,FR-601);JWT 相关不在此列
// @Tags settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SettingsResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/settings [get]
func (h *SettingsHandler) Get() gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := h.settings.All(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusOK, SettingsResponse{Settings: view})
	}
}

// Update godoc
// @Summary 更新系统设置
// @Description 白名单键值校验后持久化;登录锁定参数立即生效,路径类下次操作生效(FR-601)
// @Tags settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.UpdateSettingsBody true "设置键值"
// @Success 200 {object} handler.SettingsResponse
// @Failure 400 {object} apierr.ErrorResponse "键未知或值非法"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/settings [put]
func (h *SettingsHandler) Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body UpdateSettingsBody
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		if err := h.settings.Update(c.Request.Context(), body.Settings); err != nil {
			var invalid *service.InvalidInputError
			if errors.As(err, &invalid) {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeSettingInvalid,
					i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
				return
			}
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeSettingInvalid,
				i18n.MsgBadRequest, map[string]any{"reason": err.Error()}, err.Error())
			return
		}
		view, err := h.settings.All(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusOK, SettingsResponse{Settings: view})
	}
}
