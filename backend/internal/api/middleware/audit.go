package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/model"
)

// apiPrefixes 记录审计时从 FullPath 剥离的路由前缀。
const apiPrefix = "/api/v1/"

// AuditRepo 审计中间件对数据层的依赖;由 store 实现。
type AuditRepo interface {
	CreateAuditLog(ctx context.Context, log *model.AuditLog) error
}

// Audit 请求级审计:谁(user)、何时、调了什么 API、结果码。
// 本中间件是 audit_logs 表的唯一写入方;GET/HEAD/OPTIONS 不记录。
// 写库失败仅记日志,不影响业务响应。
func Audit(repo AuditRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return
		}

		entry := &model.AuditLog{
			Action: auditAction(c),
			Target: c.GetString(CtxKeyAuditTarget),
			NodeID: auditNodeID(c),
			IP:     c.ClientIP(),
			Detail: `{"status":` + strconv.Itoa(c.Writer.Status()) + `}`,
		}
		if id, ok := c.Get(CtxKeyUserID); ok {
			if userID, ok := id.(uint); ok && userID > 0 {
				entry.UserID = &userID // API Token 主体(user 0)审计记空,Username 已含 token 标识
			}
		}

		if err := repo.CreateAuditLog(c.Request.Context(), entry); err != nil {
			slog.Error("写入审计日志失败", "action", entry.Action, "error", err)
		}
	}
}

// auditNodeID 目标资源所属节点:handler 在处理前 Set(CtxKeyAuditNodeID, nodeID);
// 未设置(管理面操作)回落 default 'local'(DB 列默认)。
func auditNodeID(c *gin.Context) string {
	return c.GetString(CtxKeyAuditNodeID)
}

// auditAction 归一化动作标识:"/api/v1/auth/login" → "auth.login";
// 未匹配路由(FullPath 为空)退回原始路径。
// 含路由参数的路径(如 "/users/:id")在尾部追加 HTTP 方法区分操作
// ("users.:id.patch"/"users.:id.delete"),避免同路径不同动作无法区分(验收 I4-D15)。
func auditAction(c *gin.Context) string {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	path = strings.TrimPrefix(path, apiPrefix)
	action := strings.ReplaceAll(path, "/", ".")
	if strings.Contains(path, ":") {
		action += "." + strings.ToLower(c.Request.Method)
	}
	return action
}
