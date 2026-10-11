// Package handler — 日志查看接口(FR-501/502)。
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// LogsHandler 日志接口。
type LogsHandler struct {
	logs *service.Logs
}

// NewLogs 构造日志 handler。
func NewLogs(logs *service.Logs) *LogsHandler {
	return &LogsHandler{logs: logs}
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
		result, err := h.logs.Tail(c.Request.Context(), c.Query("type"), lines)
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
