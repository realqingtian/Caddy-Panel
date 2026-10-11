// Package handler — 个人资料/头像/改密(M4 自助链路)。
package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// ProfileHandler 个人资料接口。
type ProfileHandler struct {
	users  *service.UserService
	lookup UserLookup
	avatar *service.AvatarStore
}

// UserLookup 用户读取依赖(store)。
type UserLookup interface {
	GetUserByID(ctx context.Context, id uint) (*model.User, error)
}

// NewProfile 构造 profile handler。
func NewProfile(users *service.UserService, lookup UserLookup, avatar *service.AvatarStore) *ProfileHandler {
	return &ProfileHandler{users: users, lookup: lookup, avatar: avatar}
}

// ProfileResponse 个人资料响应(Header 与个人设置页数据源)。
type ProfileResponse struct {
	Username           string `json:"username"`
	Nickname           string `json:"nickname"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	AvatarURL          string `json:"avatar_url"`
	TOTPEnabled        bool   `json:"totp_enabled"`
	MustChangePassword bool   `json:"must_change_password"`
}

// Profile godoc
// @Summary 本人资料
// @Description 昵称/邮箱/头像/角色等;个人设置页与 Header 展示数据源
// @Tags profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.ProfileResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/profile [get]
func (h *ProfileHandler) Get() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := h.lookup.GetUserByID(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID))
		if err != nil {
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
			return
		}
		c.JSON(http.StatusOK, ProfileResponse{
			Username:           user.Username,
			Nickname:           user.Nickname,
			Email:              user.Email,
			Role:               user.Role,
			AvatarURL:          h.avatar.URLPath(user.AvatarPath),
			TOTPEnabled:        user.TOTPEnabled,
			MustChangePassword: user.MustChangePassword,
		})
	}
}

// PatchProfileRequest 资料维护请求(昵称/邮箱)。
type PatchProfileRequest struct {
	Nickname string `json:"nickname" example:"管理员"`
	Email    string `json:"email" example:"admin@example.com"`
}

// PatchProfile godoc
// @Summary 维护昵称/邮箱
// @Description 自助资料编辑;不涉及角色与凭证,不触发会话吊销
// @Tags profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.PatchProfileRequest true "昵称与邮箱"
// @Success 200 {object} handler.ProfileResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/profile [patch]
func (h *ProfileHandler) PatchProfile() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req PatchProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		if err := h.users.UpdateProfile(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID), req.Nickname, req.Email); err != nil {
			respondProfileError(c, err)
			return
		}
		h.Get()(c)
	}
}

// ChangePasswordRequest 修改密码请求体。
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required" example:"new-secret-8"`
}

// ChangePassword godoc
// @Summary 修改密码
// @Description 新密码 ≥8 字符且不得与旧密码相同;成功吊销本人全部会话(含当前),需重新登录
// @Tags profile
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.ChangePasswordRequest true "旧密码与新密码"
// @Success 200 {object} handler.SimpleOK
// @Failure 400 {object} apierr.ErrorResponse "策略不满足或旧密码错误"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/change-password [post]
func (h *ProfileHandler) ChangePassword() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ChangePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		if err := h.users.ChangePassword(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID), req.OldPassword, req.NewPassword); err != nil {
			respondProfileError(c, err)
			return
		}
		c.JSON(http.StatusOK, SimpleOK{OK: true})
	}
}

// PostAvatar godoc
// @Summary 上传自定义头像
// @Description multipart 上传;png/jpeg/webp(内容嗅探),≤2MiB;替换旧文件;不依赖外网
// @Tags profile
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param file formData file true "图片文件"
// @Success 200 {object} handler.ProfileResponse
// @Failure 400 {object} apierr.ErrorResponse "类型或大小不符"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/avatar [post]
func (h *ProfileHandler) PostAvatar() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint(middleware.CtxKeyUserID)
		user, err := h.lookup.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
			return
		}
		file, err := c.FormFile("file")
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		name, err := h.avatar.Save(userID, file)
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		old := user.AvatarPath
		user.AvatarPath = name
		if err := h.users.SaveAvatarRef(c.Request.Context(), userID, name); err != nil {
			_ = h.avatar.Remove(name)
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		if old != "" && old != name {
			_ = h.avatar.Remove(old)
		}
		h.Get()(c)
	}
}

// DeleteAvatar godoc
// @Summary 移除自定义头像
// @Description 删除文件并清字段,回退 Gravatar/首字母
// @Tags profile
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.ProfileResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/avatar [delete]
func (h *ProfileHandler) DeleteAvatar() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint(middleware.CtxKeyUserID)
		user, err := h.lookup.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
			return
		}
		if err := h.avatar.Remove(user.AvatarPath); err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		if err := h.users.SaveAvatarRef(c.Request.Context(), userID, ""); err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		h.Get()(c)
	}
}

// respondProfileError 业务错误映射。
func respondProfileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
	case errors.Is(err, service.ErrInvalidCredentials):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeInvalidCredentials, i18n.MsgInvalidCredentials, nil, nil)
	case errors.Is(err, service.ErrWeakPassword):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
	default:
		slog.Error("个人资料操作失败", "error", err)
		apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
	}
}
