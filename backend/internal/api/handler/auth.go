// Package handler — 登录/二段登录/登出/当前用户。
package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// AuthHandler 登录与会话接口。
type AuthHandler struct {
	auth *service.Auth
}

// NewAuth 构造 auth handler。
func NewAuth(auth *service.Auth) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// LoginRequest 登录请求体。
type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"secret"`
}

// LoginResponse 登录成功响应体(直登与二段一致)。
type LoginResponse struct {
	Token              string `json:"token"`
	Username           string `json:"username" example:"admin"`
	Nickname           string `json:"nickname" example:"管理员"`
	Role               string `json:"role" example:"admin"`
	MustChangePassword bool   `json:"must_change_password"`
}

// TwoFAResponse 需要两步验证响应体。
type TwoFAResponse struct {
	Require2FA bool   `json:"require_2fa"`
	Ticket     string `json:"ticket"`
}

// Login godoc
// @Summary 登录
// @Description 校验用户名密码;已启用两步验证的用户返回 require_2fa 与 ticket(走 /auth/login/2fa);
// @Description 连续失败达到阈值将临时锁定来源 IP(FR-101)
// @Tags auth
// @Accept json
// @Produce json
// @Param body body handler.LoginRequest true "登录凭证"
// @Success 200 {object} handler.LoginResponse
// @Success 200 {object} handler.TwoFAResponse "需要两步验证"
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 401 {object} apierr.ErrorResponse "凭证错误/账号已禁用"
// @Failure 429 {object} apierr.ErrorResponse "登录来源被锁定"
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}

		ua := c.Request.UserAgent()
		result, err := h.auth.Login(c.Request.Context(), req.Username, req.Password, c.ClientIP(), ua)
		var locked *service.LockedError
		var twoFA *service.TwoFARequiredError
		switch {
		case errors.As(err, &locked):
			c.Header("Retry-After", secondsString(locked.RetryAfter))
			apierr.RespondError(c, http.StatusTooManyRequests, apierr.CodeAccountLocked, i18n.MsgAccountLocked,
				map[string]any{"seconds": int(locked.RetryAfter.Seconds()) + 1}, nil)
		case errors.As(err, &twoFA):
			c.JSON(http.StatusOK, TwoFAResponse{Require2FA: true, Ticket: twoFA.Ticket})
		case errors.Is(err, service.ErrAccountDisabled):
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeAccountDisabled, i18n.MsgAccountDisabled, nil, nil)
		case errors.Is(err, service.ErrInvalidCredentials):
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeInvalidCredentials, i18n.MsgInvalidCredentials, nil, nil)
		case err != nil:
			slog.Error("登录失败(服务端错误)", "error", err)
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
		default:
			respondLogin(c, result)
		}
	}
}

// Login2FARequest 两步验证二段登录请求体。
type Login2FARequest struct {
	Ticket string `json:"ticket" binding:"required"`
	Code   string `json:"code" binding:"required" example:"123456"`
}

// Login2FA godoc
// @Summary 两步验证登录
// @Description ticket + 6 位验证码(或恢复码)换取 JWT;验证码错误计入登录锁定
// @Tags auth
// @Accept json
// @Produce json
// @Param body body handler.Login2FARequest true "票据与验证码"
// @Success 200 {object} handler.LoginResponse
// @Failure 401 {object} apierr.ErrorResponse "票据无效或验证码错误"
// @Failure 429 {object} apierr.ErrorResponse "登录来源被锁定"
// @Router /api/v1/auth/login/2fa [post]
func (h *AuthHandler) Login2FA() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req Login2FARequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		ua := c.Request.UserAgent()
		result, err := h.auth.Login2FA(c.Request.Context(), req.Ticket, req.Code, c.ClientIP(), ua)
		var locked *service.LockedError
		switch {
		case errors.As(err, &locked):
			c.Header("Retry-After", secondsString(locked.RetryAfter))
			apierr.RespondError(c, http.StatusTooManyRequests, apierr.CodeAccountLocked, i18n.MsgAccountLocked,
				map[string]any{"seconds": int(locked.RetryAfter.Seconds()) + 1}, nil)
		case errors.Is(err, service.ErrTwoFATicketInvalid), errors.Is(err, service.ErrInvalidTwoFACode):
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeTwoFAInvalid, i18n.MsgTwoFAInvalid, nil, nil)
		case errors.Is(err, service.ErrAccountDisabled), errors.Is(err, service.ErrInvalidCredentials):
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeInvalidCredentials, i18n.MsgInvalidCredentials, nil, nil)
		case err != nil:
			slog.Error("2FA 登录失败(服务端错误)", "error", err)
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
		default:
			respondLogin(c, result)
		}
	}
}

// Logout godoc
// @Summary 退出登录
// @Description 服务端吊销当前会话(M4 起退出为服务端动作,旧 token 立即失效)
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SimpleOK
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		if sid, ok := c.Get(middleware.CtxKeySID); ok {
			if s, ok := sid.(string); ok {
				if err := h.auth.LogoutSession(c.Request.Context(), s); err != nil {
					slog.Error("退出登录吊销会话失败", "error", err)
				}
			}
		}
		c.JSON(http.StatusOK, SimpleOK{OK: true})
	}
}

// SimpleOK 通用成功响应。
type SimpleOK struct {
	OK bool `json:"ok"`
}

// respondLogin 登录成功统一响应(直登与二段共用;审计需要身份先行注入)。
func respondLogin(c *gin.Context, result *service.LoginResult) {
	c.Set(middleware.CtxKeyUserID, result.UserID)
	c.JSON(http.StatusOK, LoginResponse{
		Token:              result.Token,
		Username:           result.Username,
		Nickname:           result.Nickname,
		Role:               result.Role,
		MustChangePassword: result.MustChangePassword,
	})
}

// MeResponse 当前用户响应体。
type MeResponse struct {
	UserID   uint   `json:"id" example:"1"`
	Username string `json:"username" example:"admin"`
	Role     string `json:"role" example:"admin"`
}

// Me godoc
// @Summary 当前用户
// @Description 返回 JWT 对应的面板用户身份
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.MeResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/me [get]
func (h *AuthHandler) Me() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, MeResponse{
			UserID:   c.GetUint(middleware.CtxKeyUserID),
			Username: c.GetString(middleware.CtxKeyUsername),
			Role:     c.GetString(middleware.CtxKeyRole),
		})
	}
}

// secondsString 锁定剩余时长转秒(向上取整,保证 Retry-After 不会为 0)。
func secondsString(d time.Duration) string {
	return strconv.Itoa(int(d.Seconds()) + 1)
}
