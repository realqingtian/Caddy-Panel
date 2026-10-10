package middleware

import (
	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/i18n"
)

// Language 解析 Accept-Language 并写入 context;业务与错误响应据此选语言(FR-603)。
func Language() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(CtxKeyLang, i18n.NormalizeLang(c.GetHeader("Accept-Language")))
		c.Next()
	}
}
