// Package handler — 两步验证自助管理(M4)。
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// TwoFAHandler 两步验证接口。
type TwoFAHandler struct {
	svc *service.TOTPService
}

// NewTwoFA 构造 2FA handler。
func NewTwoFA(svc *service.TOTPService) *TwoFAHandler {
	return &TwoFAHandler{svc: svc}
}

// SetupResponse 绑定准备响应(secret 仅本次返回,前端渲染二维码)。
type SetupResponse struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

// EnableRequest 启用请求(secret 来自 setup 响应 + 验证码确认)。
type EnableRequest struct {
	Secret string `json:"secret" binding:"required"`
	Code   string `json:"code" binding:"required" example:"123456"`
}

// RecoveryResponse 恢复码响应(明文仅此一次)。
type RecoveryResponse struct {
	Codes []string `json:"codes"`
}

// Setup godoc
// @Summary 生成两步验证绑定信息
// @Description 返回 TOTP 密钥与 otpauth URI(前端渲染二维码);确认前不落库
// @Tags twofa
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SetupResponse
// @Failure 400 {object} apierr.ErrorResponse "已启用"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/2fa/setup [post]
func (h *TwoFAHandler) Setup() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret, uri, err := h.svc.Setup(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID))
		if err != nil {
			respondTwoFAError(c, err)
			return
		}
		c.JSON(http.StatusOK, SetupResponse{Secret: secret, URI: uri})
	}
}

// Enable godoc
// @Summary 确认启用两步验证
// @Description 验证码校验通过才落库启用,返回 10 个恢复码(明文仅此一次)
// @Tags twofa
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.EnableRequest true "密钥与验证码"
// @Success 200 {object} handler.RecoveryResponse
// @Failure 400 {object} apierr.ErrorResponse "验证码错误或已启用"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/2fa/enable [post]
func (h *TwoFAHandler) Enable() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req EnableRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		codes, err := h.svc.Enable(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID), req.Secret, req.Code)
		if err != nil {
			respondTwoFAError(c, err)
			return
		}
		c.JSON(http.StatusOK, RecoveryResponse{Codes: codes})
	}
}

// DisableRequest 解绑请求(密码 + 验证码双因子)。
type DisableRequest struct {
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required" example:"123456"`
}

// Disable godoc
// @Summary 解绑两步验证
// @Description 需当前密码 + 当前验证码;成功吊销全部会话
// @Tags twofa
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.DisableRequest true "密码与验证码"
// @Success 200 {object} handler.SimpleOK
// @Failure 400 {object} apierr.ErrorResponse "密码或验证码错误"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/2fa [delete]
func (h *TwoFAHandler) Disable() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req DisableRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		if err := h.svc.Disable(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID), req.Password, req.Code); err != nil {
			respondTwoFAError(c, err)
			return
		}
		c.JSON(http.StatusOK, SimpleOK{OK: true})
	}
}

// RecoveryCodeRequest 重新生成恢复码请求(需当前验证码)。
type RecoveryCodeRequest struct {
	Code string `json:"code" binding:"required" example:"123456"`
}

// RegenerateRecovery godoc
// @Summary 重新生成恢复码
// @Description 需当前验证码;旧恢复码全部作废,返回新 10 个(明文仅此一次)
// @Tags twofa
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.RecoveryCodeRequest true "当前验证码"
// @Success 200 {object} handler.RecoveryResponse
// @Failure 400 {object} apierr.ErrorResponse "验证码错误或未启用"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/2fa/recovery [post]
func (h *TwoFAHandler) RegenerateRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RecoveryCodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		codes, err := h.svc.RegenerateRecoveryCodes(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID), req.Code)
		if err != nil {
			respondTwoFAError(c, err)
			return
		}
		c.JSON(http.StatusOK, RecoveryResponse{Codes: codes})
	}
}

// respondTwoFAError 业务错误映射。
func respondTwoFAError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidTwoFACode):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeTwoFAInvalid, i18n.MsgTwoFAInvalid, nil, nil)
	case errors.Is(err, service.ErrUserNotFound):
		apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
	default:
		slog.Error("两步验证操作失败", "error", err)
		apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
	}
}
