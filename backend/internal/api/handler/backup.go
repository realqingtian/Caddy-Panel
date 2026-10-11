// Package handler — 备份与恢复接口(FR-706)。
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/service"
)

// BackupHandler 备份恢复接口。
type BackupHandler struct {
	backup *service.BackupService
}

// NewBackup 构装备份恢复 handler。
func NewBackup(backup *service.BackupService) *BackupHandler {
	return &BackupHandler{backup: backup}
}

// Export godoc
// @Summary 导出备份
// @Description 下载站点/版本历史/设置的全量 JSON 备份(FR-706)
// @Tags backup
// @Produce json
// @Security BearerAuth
// @Success 200 {object} service.BackupFile
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/backup [get]
func (h *BackupHandler) Export() gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := h.backup.Export(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		data, err := file.Marshal()
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		filename := "caddy-panel-backup-" + time.Now().Format("20060102-150405") + ".json"
		c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
		c.Data(http.StatusOK, "application/json", data)
	}
}

// Restore godoc
// @Summary 导入备份
// @Description 事务内替换站点/版本历史/设置;import_users=true 时增量导入备份中的用户与节点(同名/同 ID 跳过,不覆盖现网账号);不自动下发,恢复后需重新启用或下发(FR-706/I4-D9)
// @Tags backup
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body service.BackupFile true "备份文件内容"
// @Param import_users query bool false "同时导入 users/nodes(纯增量)" default(false)
// @Success 200 {object} service.RestoreResult
// @Failure 400 {object} apierr.ErrorResponse "备份格式或数据非法"
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/restore [post]
func (h *BackupHandler) Restore() gin.HandlerFunc {
	return func(c *gin.Context) {
		var file service.BackupFile
		if err := c.ShouldBindJSON(&file); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		result, err := h.backup.Restore(c.Request.Context(), &file, c.Query("import_users") == "true")
		if err != nil {
			var invalid *service.InvalidInputError
			if errors.As(err, &invalid) {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
					i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
				return
			}
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
