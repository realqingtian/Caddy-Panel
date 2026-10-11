// Package handler — 日志查看接口(FR-501/502)。
package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// LogsHandler 日志接口。
type LogsHandler struct {
	logs      *service.Logs
	jwtSecret []byte
	sessions  SessionVerifier
	users     UserLookup
}

// NewLogs 构造日志 handler。
func NewLogs(logs *service.Logs, jwtSecret []byte, sessions SessionVerifier, users UserLookup) *LogsHandler {
	return &LogsHandler{logs: logs, jwtSecret: jwtSecret, sessions: sessions, users: users}
}

// TailResponse 日志 tail 响应。
type TailResponse = service.TailResult

// Tail godoc
// @Summary 日志末尾 N 行
// @Description type=access|error;文件不存在时 exists=false 而非报错(FR-501/502)
// @Tags logs
// @Produce json
// @Security BearerAuth
// @Param type query string true "日志类型" Enums(access,error)
// @Param tail query int false "行数(默认 200,上限 1000)" default(200)
// @Param keyword query string false "关键字过滤(整行子串,忽略大小写;I7)"
// @Param status query string false "状态码前缀过滤(逗号分隔,如 4,5;仅 JSON 行;I7)"
// @Success 200 {object} handler.TailResponse
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/logs [get]
func (h *LogsHandler) Tail() gin.HandlerFunc {
	return func(c *gin.Context) {
		lines, err := strconv.Atoi(c.DefaultQuery("tail", "200"))
		if err != nil {
			lines = 200
		}
		result, err := h.logs.Tail(c.Request.Context(), c.Query("type"), lines, c.Query("keyword"), c.Query("status"))
		if err != nil {
			var invalid *service.InvalidInputError
			if errors.As(err, &invalid) {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
					i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
				return
			}
			// 读取失败(常见为跨容器文件权限)返回带原因的可读消息而非笼统 500(I4-D2)
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal,
				i18n.MsgLogReadFailed, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, result)
	}
}


// LogStream godoc
// @Summary 日志实时流(SSE)
// @Description 增量推送新日志行(500ms 轮询增量);token 走查询参数(与终端 WS 同模式);页面隐藏时前端应关闭连接(I7)
// @Tags logs
// @Produce text/event-stream
// @Security BearerAuth
// @Param token query string true "JWT(登录获得的会话令牌)"
// @Param type query string true "日志类型" Enums(access,error)
// @Param keyword query string false "关键字过滤(同 /logs)"
// @Param status query string false "状态码前缀过滤(同 /logs)"
// @Success 200 {string} string "text/event-stream"
// @Failure 401 {string} string "未认证"
// @Router /api/v1/logs/stream [get]
func (h *LogsHandler) Stream() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 认证与终端 WS 同模式:SSE 无法自定义请求头,JWT 走查询参数;会话与账号状态双检。
		claims, err := service.ParseJWT(h.jwtSecret, c.Query("token"))
		if err != nil || claims.SID == "" {
			c.String(http.StatusUnauthorized, "unauthorized")
			return
		}
		if _, err := h.sessions.Verify(c.Request.Context(), claims.SID); err != nil {
			c.String(http.StatusUnauthorized, "unauthorized")
			return
		}
		user, err := h.users.GetUserByID(c.Request.Context(), claims.UserID)
		if err != nil || user.Disabled || user.MustChangePassword {
			c.String(http.StatusForbidden, "forbidden")
			return
		}
		kind := c.Query("type")
		if kind != "access" && kind != "error" {
			kind = "access"
		}
		keyword, status := c.Query("keyword"), c.Query("status")

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			return
		}

		// 从当前文件末尾订阅:实时流只推连接之后的新行;从 0 回放会淹没前端
		// 并对大文件整读(I7 验收发现的历史回放缺陷)。
		offset := h.logs.Size(kind)
		if data, off, exists, err := service.ReadNew(h.logs.Path(kind), offset); err == nil && exists && len(data) > 0 {
			writeSSELines(c, flusher, service.FilterLogLines(strings.Split(string(data), "\n"), keyword, status))
			offset = off
		}

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-ticker.C:
				data, off, _, err := service.ReadNew(h.logs.Path(kind), offset)
				if err != nil || len(data) == 0 {
					continue
				}
				offset = off
				writeSSELines(c, flusher, service.FilterLogLines(strings.Split(string(data), "\n"), keyword, status))
			}
		}
	}
}

// writeSSELines 以 SSE event:line 帧写出日志行。
func writeSSELines(c *gin.Context, flusher http.Flusher, lines []string) {
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		c.String(http.StatusOK, "event: line\ndata: %s\n\n", line)
	}
	flusher.Flush()
}