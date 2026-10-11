// Package handler — 用户管理(admin,users:manage)。
package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// UserHandler 用户管理接口。
type UserHandler struct {
	users    *service.UserService
	sessions service.SessionLister
}

// NewUser 构造用户管理 handler。
func NewUser(users *service.UserService, sessions service.SessionLister) *UserHandler {
	return &UserHandler{users: users, sessions: sessions}
}

// ListUsers godoc
// @Summary 用户列表
// @Description 全部存活用户;含角色/状态/两步验证/会话数/最后活跃
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {array} service.UserView
// @Failure 403 {object} apierr.ErrorResponse
// @Router /api/v1/users [get]
func (h *UserHandler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		views, err := h.users.List(c.Request.Context(), h.sessions)
		if err != nil {
			slog.Error("查询用户列表失败", "error", err)
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, views)
	}
}

// CreateUserRequest 创建用户请求体。
type CreateUserRequest struct {
	Username           string `json:"username" binding:"required" example:"editor01"`
	Password           string `json:"password" binding:"required" example:"initial-pass"`
	Nickname           string `json:"nickname" example:"编辑一号"`
	Email              string `json:"email" example:"editor@example.com"`
	Role               string `json:"role" binding:"required" example:"editor"`
	MustChangePassword *bool  `json:"must_change_password"` // 缺省 true:首登强制改密
}

// Create godoc
// @Summary 创建用户
// @Description admin 创建;初始密码首登强制改密(可通过 must_change_password 显式关闭)
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.CreateUserRequest true "用户信息"
// @Success 200 {object} service.UserView
// @Failure 400 {object} apierr.ErrorResponse "用户名冲突/角色非法/密码过弱"
// @Failure 403 {object} apierr.ErrorResponse
// @Router /api/v1/users [post]
func (h *UserHandler) Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		mustChange := true
		if req.MustChangePassword != nil {
			mustChange = *req.MustChangePassword
		}
		view, err := h.users.Create(c.Request.Context(), service.CreateUserRequest{
			Username:           req.Username,
			Password:           req.Password,
			Nickname:           req.Nickname,
			Email:              req.Email,
			Role:               req.Role,
			MustChangePassword: mustChange,
		})
		if err != nil {
			respondUserError(c, err)
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

// UpdateUserRequest 更新用户请求体(字段按需提供)。
type UpdateUserRequest struct {
	Nickname    *string `json:"nickname"`
	Email       *string `json:"email"`
	Role        *string `json:"role" example:"viewer"`
	Disabled    *bool   `json:"disabled"`
	NewPassword *string `json:"new_password" example:"new-pass-8"`
}

// Update godoc
// @Summary 修改用户
// @Description 角色/禁用/重置密码变更即吊销目标用户全部会话;昵称/邮箱为资料维护(不吊销)
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Param body body handler.UpdateUserRequest true "变更字段"
// @Success 200 {object} service.UserView
// @Failure 400 {object} apierr.ErrorResponse "保护规则/角色非法"
// @Failure 403 {object} apierr.ErrorResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/users/{id} [patch]
func (h *UserHandler) Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		var req UpdateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		view, err := h.users.Update(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID), uint(id), service.UpdateRequest{
			Nickname:    req.Nickname,
			Email:       req.Email,
			Role:        req.Role,
			Disabled:    req.Disabled,
			NewPassword: req.NewPassword,
		})
		if err != nil {
			respondUserError(c, err)
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

// Delete godoc
// @Summary 删除用户
// @Description 软删除(审计保留);吊销全部会话;最后一个可用 admin 受保护
// @Tags users
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} handler.SimpleOK
// @Failure 400 {object} apierr.ErrorResponse "保护规则"
// @Failure 403 {object} apierr.ErrorResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/users/{id} [delete]
func (h *UserHandler) Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		if err := h.users.Delete(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID), uint(id)); err != nil {
			respondUserError(c, err)
			return
		}
		c.JSON(http.StatusOK, SimpleOK{OK: true})
	}
}

// respondUserError 用户管理业务错误映射。
func respondUserError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		apierr.RespondError(c, http.StatusNotFound, apierr.CodeUserNotFound, i18n.MsgUserNotFound, nil, nil)
	case errors.Is(err, service.ErrUsernameConflict):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeUsernameConflict, i18n.MsgUsernameConflict, nil, nil)
	case errors.Is(err, service.ErrSelfOperation):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, "不能对当前登录账号执行该操作")
	case errors.Is(err, service.ErrLastAdminProtected):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, "系统至少保留一个可用 admin")
	case errors.Is(err, service.ErrInvalidRole), errors.Is(err, service.ErrWeakPassword):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
	default:
		slog.Error("用户管理操作失败", "error", err)
		apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
	}
}
