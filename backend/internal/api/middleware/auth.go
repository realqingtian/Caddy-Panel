// Package middleware — 双轨认证(JWT 会话 / 开放 API Token)与 scope 校验。
package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// TokenAuthenticator API Token 认证依赖;由 service.TokenService 实现。
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, plain string) (*model.ApiToken, error)
}

// Authenticate 双轨认证中间件:
//   - Bearer cpt_*:API Token(sha256 查找 + 过期检查),scope 取 token scopes;
//   - Bearer JWT:现有会话,scope 由角色推导(admin→write,viewer→read;历史空 role 视为 admin)。
//
// 认证成功后注入 scope/auth_kind/身份;失败一律 401(由 unauthorized 回调按语言给文案)。
func Authenticate(
	jwtSecret []byte,
	tokens TokenAuthenticator,
	unauthorized func(c *gin.Context, status int),
) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		token := strings.TrimPrefix(raw, bearerPrefix)
		if token == "" || token == raw {
			unauthorized(c, 401)
			c.Abort()
			return
		}

		// API Token 轨道。
		if strings.HasPrefix(token, service.TokenPrefix) {
			apiToken, err := tokens.Authenticate(c.Request.Context(), token)
			if err != nil {
				unauthorized(c, 401)
				c.Abort()
				return
			}
			c.Set(CtxKeyUserID, uint(0)) // 机器主体:审计记录 UserID 为空
			c.Set(CtxKeyUsername, "token:"+apiToken.Name)
			c.Set(CtxKeyScope, apiToken.Scopes)
			c.Set(CtxKeyAuthKind, "api_token")
			c.Next()
			return
		}

		// JWT 会话轨道。
		claims, err := service.ParseJWT(jwtSecret, token)
		if err != nil {
			unauthorized(c, 401)
			c.Abort()
			return
		}
		role := claims.Role
		if role == "" {
			role = model.RoleAdmin // 历史会话无 role 字段:保持既有行为
		}
		scope := model.ScopeWrite
		if role == model.RoleViewer {
			scope = model.ScopeRead
		}
		c.Set(CtxKeyUserID, claims.UserID)
		c.Set(CtxKeyUsername, claims.Username)
		c.Set(CtxKeyRole, role)
		c.Set(CtxKeyScope, scope)
		c.Set(CtxKeyAuthKind, "jwt")
		c.Next()
	}
}

// RequireScope 校验已认证主体的 scope 满足要求(write ⊇ read);
// 不满足返回 403(forbidden 回调按语言给文案)。
func RequireScope(required string, forbidden func(c *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		granted := c.GetString(CtxKeyScope)
		if granted == required || granted == model.ScopeWrite {
			c.Next()
			return
		}
		forbidden(c)
		c.Abort()
	}
}

// RequireJWTAdmin 仅允许"JWT + admin 角色"访问(API Token 不得自我管理)。
func RequireJWTAdmin(forbidden func(c *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(CtxKeyAuthKind) != "jwt" || c.GetString(CtxKeyRole) != model.RoleAdmin {
			forbidden(c)
			c.Abort()
			return
		}
		c.Next()
	}
}
