// Package handler — 开放 API Token 管理接口(M3;仅 JWT + admin 可访问)。
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// TokenHandler API Token 管理接口。
type TokenHandler struct {
	tokens *service.TokenService
}

// NewToken 构造 token 管理 handler。
func NewToken(tokens *service.TokenService) *TokenHandler {
	return &TokenHandler{tokens: tokens}
}

// CreateTokenBody 创建请求体。
type CreateTokenBody struct {
	Name      string `json:"name" binding:"required"`
	Scope     string `json:"scope" binding:"required,oneof=read write" example:"read"`
	ExpiresIn string `json:"expires_in" example:"720h"`
}

// CreateTokenResponse 创建响应;明文仅此一次返回。
type CreateTokenResponse struct {
	PlainToken string         `json:"plain_token"`
	Token      model.ApiToken `json:"token"`
}

// List godoc
// @Summary API Token 列表
// @Description 返回全部 token(仅哈希与前缀,无明文)
// @Tags tokens
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.ApiToken
// @Failure 401 {object} apierr.ErrorResponse
// @Failure 403 {object} apierr.ErrorResponse
// @Router /api/v1/tokens [get]
func (h *TokenHandler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokens, err := h.tokens.List(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusOK, tokens)
	}
}

// Create godoc
// @Summary 签发 API Token
// @Description 明文仅本次返回,库内只存哈希;expires_in 为 Go duration(如 720h,留空不过期)
// @Tags tokens
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.CreateTokenBody true "Token 定义"
// @Success 201 {object} handler.CreateTokenResponse
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 403 {object} apierr.ErrorResponse
// @Router /api/v1/tokens [post]
func (h *TokenHandler) Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body CreateTokenBody
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		var expiresIn time.Duration
		if body.ExpiresIn != "" {
			var parseErr error
			expiresIn, parseErr = time.ParseDuration(body.ExpiresIn)
			if parseErr != nil {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest,
					map[string]any{"reason": "expires_in 格式非法(Go duration)"}, parseErr.Error())
				return
			}
		}
		result, err := h.tokens.Create(c.Request.Context(), body.Name, body.Scope, expiresIn)
		if err != nil {
			var invalid *service.InvalidInputError
			if errors.As(err, &invalid) {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
					i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
				return
			}
			if errors.Is(err, service.ErrTokenBadScopes) {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
					i18n.MsgBadRequest, map[string]any{"reason": "scope 必须为 read 或 write"}, nil)
				return
			}
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusCreated, CreateTokenResponse{PlainToken: result.PlainToken, Token: *result.Token})
	}
}

// Delete godoc
// @Summary 删除 API Token
// @Tags tokens
// @Security BearerAuth
// @Param id path int true "Token ID"
// @Success 204
// @Failure 403 {object} apierr.ErrorResponse
// @Router /api/v1/tokens/{id} [delete]
func (h *TokenHandler) Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if err := h.tokens.Delete(c.Request.Context(), id); err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
