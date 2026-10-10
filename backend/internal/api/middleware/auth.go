package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/service"
)

// bearerPrefix Authorization 头的 Bearer 方案前缀。
const bearerPrefix = "Bearer "

// JWTAuth 校验 Bearer Token 并注入用户身份;失败一律 401(由 i18n 按语言给文案)。
func JWTAuth(jwtSecret []byte, unauthorized func(c *gin.Context, code int)) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		token := strings.TrimPrefix(raw, bearerPrefix)
		if token == "" || token == raw {
			unauthorized(c, 401)
			c.Abort()
			return
		}
		claims, err := service.ParseJWT(jwtSecret, token)
		if err != nil {
			unauthorized(c, 401)
			c.Abort()
			return
		}
		c.Set(CtxKeyUserID, claims.UserID)
		c.Set(CtxKeyUsername, claims.Username)
		c.Next()
	}
}
