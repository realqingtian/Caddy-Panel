// Package middleware — 双轨认证(JWT 会话 / 开放 API Token)与权限点校验。
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// TokenAuthenticator API Token 认证依赖;由 service.TokenService 实现。
type TokenAuthenticator interface {
	Authenticate(ctx context.Context, plain string) (*model.ApiToken, error)
}

// SessionVerifier 会话校验依赖(*service.SessionService)。
type SessionVerifier interface {
	Verify(ctx context.Context, sid string) (*model.Session, error)
	Touch(ctx context.Context, id uint)
}

// UserLookup 用户读取依赖(会话 → 用户状态联查)。
type UserLookup interface {
	GetUserByID(ctx context.Context, id uint) (*model.User, error)
}

// AuthDeps 认证中间件依赖;main 装配。
type AuthDeps struct {
	JWTSecret    []byte
	Tokens       TokenAuthenticator
	Sessions     SessionVerifier
	Users        UserLookup
	Unauthorized func(c *gin.Context, status int)
}

// Authenticate 双轨认证中间件:
//   - Bearer cpt_*:API Token,权限集由 scope 翻译(恒不含高危三点,§3.3);
//   - Bearer JWT:解析 claims(无 sid 的存量 token 一律 401,§4 存量迁移)→
//     会话双检(未吊销/未过期)→ 用户状态(存在/未禁用)→ 权限点推导。
func Authenticate(deps AuthDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		token := strings.TrimPrefix(raw, bearerPrefix)
		if token == "" || token == raw {
			deps.Unauthorized(c, 401)
			c.Abort()
			return
		}

		// API Token 轨:机器主体,无会话/2FA 语义,审计 UserID 记空。
		if strings.HasPrefix(token, service.TokenPrefix) {
			apiToken, err := deps.Tokens.Authenticate(c.Request.Context(), token)
			if err != nil {
				deps.Unauthorized(c, 401)
				c.Abort()
				return
			}
			c.Set(CtxKeyUserID, uint(0))
			c.Set(CtxKeyUsername, "token:"+apiToken.Name)
			c.Set(CtxKeyAuthKind, "api_token")
			c.Set(CtxKeyPermissions, model.TokenPermissions([]string{apiToken.Scopes}))
			c.Next()
			return
		}

		// JWT 会话轨:签名 → sid → 会话 → 用户,任一失效即 401。
		claims, err := service.ParseJWT(deps.JWTSecret, token)
		if err != nil {
			println("DEBUG ParseJWT:", err.Error())
		}
		if claims != nil {
			println("DEBUG sid:", claims.SID)
		}
		if err != nil || claims.SID == "" {
			// 无 sid = M4 之前的存量 token:会话双检无法生效,直接拒绝(§4 存量迁移)。
			deps.Unauthorized(c, 401)
			c.Abort()
			return
		}
		session, err := deps.Sessions.Verify(c.Request.Context(), claims.SID)
		if err != nil {
			println("DEBUG Verify:", err.Error())
		} else {
			println("DEBUG Verify ok, user lookup:")
		}
		if err != nil {
			deps.Unauthorized(c, 401)
			c.Abort()
			return
		}
		user, err := deps.Users.GetUserByID(c.Request.Context(), claims.UserID)
		if err != nil || user.Disabled {
			deps.Unauthorized(c, 401)
			c.Abort()
			return
		}

		role := claims.Role
		if role == "" {
			role = model.RoleAdmin // 历史会话无 role 字段:保持既有行为
		}
		c.Set(CtxKeyUserID, user.ID)
		c.Set(CtxKeyUsername, user.Username)
		c.Set(CtxKeyRole, role)
		c.Set(CtxKeySID, claims.SID)
		c.Set(CtxKeyAuthKind, "jwt")
		c.Set(CtxKeyPermissions, model.RolePermissions(role))
		c.Set(CtxKeyMustChangePass, user.MustChangePassword)
		c.Set(CtxKeyScope, scopeFromPermissions(model.RolePermissions(role))) // 兼容 M3 遗留读取点

		// 最后活跃节流:距上次 >1 分钟才落库。
		if time.Since(session.LastSeenAt) > time.Minute {
			deps.Sessions.Touch(c.Request.Context(), session.ID)
		}
		c.Next()
	}
}

// scopeFromPermissions 权限集 → M3 二值 scope(遗留读取点兼容)。
func scopeFromPermissions(perms []string) string {
	for _, p := range perms {
		if p == "*" || strings.HasSuffix(p, ":write") {
			return model.ScopeWrite
		}
	}
	return model.ScopeRead
}

// RequirePermission 权限点校验:权限集含通配 "*" 或目标点即放行,否则 403。
// 高危权限点(§3.2)仅 admin 的 "*" 包含,API Token 权限集从不派发,天然落实硬规则。
func RequirePermission(required string, forbidden func(c *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		perms, _ := c.Get(CtxKeyPermissions)
		list, _ := perms.([]string)
		if model.HasPermission(list, required) {
			c.Next()
			return
		}
		forbidden(c)
		c.Abort()
	}
}

// RequirePasswordChanged 强制改密服务端闸门:must_change_password 用户仅放行白名单路由;
// 白名单外的读接口也拦截,确保强制状态无法被绕过(§5)。白名单内路由不挂本中间件。
func RequirePasswordChanged(forbidden func(c *gin.Context, status int, code string)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetBool(CtxKeyMustChangePass) {
			// code 固定 PASSWORD_CHANGE_REQUIRED,由 router 的闭包提供(避免 apierr 循环导入)。
			forbidden(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireJWTAdmin 仅允许"JWT + admin 角色"访问(高危端点双层保险:
// 权限点校验之外,再断言认证轨道与角色;API Token 恒拒)。
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
