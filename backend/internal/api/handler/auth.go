// Package handler 存放 HTTP handler:仅做参数绑定与协议转换,业务逻辑一律在 service 层。
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

// LoginResponse 登录成功响应体。
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username" example:"admin"`
	Role     string `json:"role" example:"admin"`
}

// MeResponse 当前用户响应体。
type MeResponse struct {
	UserID   uint   `json:"id" example:"1"`
	Username string `json:"username" example:"admin"`
	Role     string `json:"role" example:"admin"`
}

// Login godoc
// @Summary 登录
// @Description 校验用户名密码,返回 JWT;连续失败达到阈值将临时锁定来源 IP(FR-101)
// @Tags auth
// @Accept json
// @Produce json
// @Param body body handler.LoginRequest true "登录凭证"
// @Success 200 {object} handler.LoginResponse
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 401 {object} apierr.ErrorResponse "用户名或密码错误"
// @Failure 429 {object} apierr.ErrorResponse "登录来源被锁定"
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}

		result, err := h.auth.Login(c.Request.Context(), req.Username, req.Password, c.ClientIP())
		var locked *service.LockedError
		switch {
		case errors.As(err, &locked):
			c.Header("Retry-After", secondsString(locked.RetryAfter))
			apierr.RespondError(c, http.StatusTooManyRequests, apierr.CodeAccountLocked, i18n.MsgAccountLocked,
				map[string]any{"seconds": int(locked.RetryAfter.Seconds()) + 1}, nil)
		case errors.Is(err, service.ErrInvalidCredentials):
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeInvalidCredentials, i18n.MsgInvalidCredentials, nil, nil)
		case err != nil:
			slog.Error("登录失败(服务端错误)", "error", err)
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
		default:
			// 登录成功同样把身份写入 context,审计中间件记录"谁在登录"。
			c.Set(middleware.CtxKeyUserID, result.UserID)
			c.JSON(http.StatusOK, LoginResponse{Token: result.Token, Username: result.Username, Role: result.Role})
		}
	}
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
