// Package handler — 登录设备(会话)管理(M4 自助 + admin 视角)。
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// SessionHandler 会话管理接口。
type SessionHandler struct {
	sessions *service.SessionService
	users    *service.UserService
}

// NewSession 构造会话 handler。
func NewSession(sessions *service.SessionService, users *service.UserService) *SessionHandler {
	return &SessionHandler{sessions: sessions, users: users}
}

// SessionView 会话响应(设备名解析 + 当前标记)。
type SessionView struct {
	SID        string `json:"sid"`
	Device     string `json:"device"`
	IP         string `json:"ip"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	Current    bool   `json:"current"`
	Revoked    bool   `json:"revoked"`
}

// deviceLabel 从 UserAgent 提取浏览器/系统简称(够辨识即可,不做完整 UA 解析)。
func deviceLabel(ua string) string {
	ua = strings.ToLower(ua)
	switch {
	case strings.Contains(ua, "edg/"):
		return "Edge"
	case strings.Contains(ua, "chrome"):
		return "Chrome"
	case strings.Contains(ua, "firefox"):
		return "Firefox"
	case strings.Contains(ua, "safari"):
		return "Safari"
	case strings.Contains(ua, "curl/"):
		return "curl"
	case strings.Contains(ua, "dart"):
		return "Dart/CLI"
	default:
		return "未知设备"
	}
}

func toSessionViews(sessions []model.Session, currentSID string) []SessionView {
	views := make([]SessionView, 0, len(sessions))
	for _, s := range sessions {
		views = append(views, SessionView{
			SID:        s.SID,
			Device:     deviceLabel(s.UserAgent),
			IP:         s.IP,
			CreatedAt:  s.CreatedAt.Format("2006-01-02 15:04"),
			LastSeenAt: s.LastSeenAt.Format("2006-01-02 15:04"),
			Current:    s.SID == currentSID,
			Revoked:    s.RevokedAt != nil,
		})
	}
	return views
}

// ListMine godoc
// @Summary 本人会话列表
// @Description 登录设备列表;当前设备置顶标"本设备"
// @Tags sessions
// @Produce json
// @Security BearerAuth
// @Success 200 {array} handler.SessionView
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/sessions [get]
func (h *SessionHandler) ListMine() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessions, err := h.sessions.ListByUser(c.Request.Context(), c.GetUint(middleware.CtxKeyUserID))
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, toSessionViews(sessions, c.GetString(middleware.CtxKeySID)))
	}
}

// DeleteMine godoc
// @Summary 下线本人指定会话
// @Description 仅限本人会话;下线后该设备下一次请求 401
// @Tags sessions
// @Produce json
// @Security BearerAuth
// @Param sid path string true "会话 ID"
// @Success 200 {object} handler.SimpleOK
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/sessions/{sid} [delete]
func (h *SessionHandler) DeleteMine() gin.HandlerFunc {
	return func(c *gin.Context) {
		sid := c.Param("sid")
		if sid == c.GetString(middleware.CtxKeySID) {
			// 下线当前会话等价退出登录,前端直接走 /auth/logout(语义更清晰)。
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, "当前会话请使用退出登录")
			return
		}
		if owner, ok := h.ownerOf(c.Request.Context(), sid); !ok || owner != c.GetUint(middleware.CtxKeyUserID) {
			apierr.RespondError(c, http.StatusNotFound, apierr.CodeNotFound, i18n.MsgNotFound, nil, nil)
			return
		}
		if err := h.sessions.Logout(c.Request.Context(), sid); err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, SimpleOK{OK: true})
	}
}

// DeleteOthers godoc
// @Summary 下线本人其他设备
// @Description 保留当前会话,吊销本人全部其他会话
// @Tags sessions
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SimpleOK
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/auth/sessions/others [post]
func (h *SessionHandler) DeleteOthers() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetUint(middleware.CtxKeyUserID)
		if err := h.sessions.RevokeUser(c.Request.Context(), userID, c.GetString(middleware.CtxKeySID)); err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, SimpleOK{OK: true})
	}
}

// ListForUser godoc
// @Summary 查看指定用户的会话(admin)
// @Description 用户管理排查视角
// @Tags users
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {array} handler.SessionView
// @Failure 403 {object} apierr.ErrorResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/users/{id}/sessions [get]
func (h *SessionHandler) ListForUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		sessions, err := h.sessions.ListByUser(c.Request.Context(), uint(userID))
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, toSessionViews(sessions, ""))
	}
}

// DeleteForUser godoc
// @Summary 强制下线指定用户的会话(admin)
// @Description admin 强制下线目标用户的指定设备
// @Tags users
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Param sid path string true "会话 ID"
// @Success 200 {object} handler.SimpleOK
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 403 {object} apierr.ErrorResponse
// @Router /api/v1/users/{id}/sessions/{sid} [delete]
func (h *SessionHandler) DeleteForUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		sid := c.Param("sid")
		if owner, ok := h.ownerOf(c.Request.Context(), sid); !ok || owner != uint(userID) {
			apierr.RespondError(c, http.StatusNotFound, apierr.CodeNotFound, i18n.MsgNotFound, nil, nil)
			return
		}
		if err := h.sessions.Logout(c.Request.Context(), sid); err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, SimpleOK{OK: true})
	}
}

// ownerOf 查会话归属(service 经 store 查询)。
func (h *SessionHandler) ownerOf(ctx context.Context, sid string) (uint, bool) {
	id, ok, err := h.sessions.OwnerOf(ctx, sid)
	if err != nil {
		return 0, false
	}
	return id, ok
}
