// Package middleware 提供 Gin 中间件:请求语言解析、双轨认证(JWT/API Token)、
// scope 校验与审计写入。
// 登录失败锁定为业务规则,实现在 service.LoginGuard,不走中间件。
package middleware

// context 键:中间件写入、handler 读取。
const (
	CtxKeyLang        = "lang"          // i18n 解析后的语言(如 zh-CN)
	CtxKeyUserID      = "auth_user_id"  // JWT 用户 ID;API Token 主体为 0(审计记为空)
	CtxKeyUsername    = "auth_username" // JWT 用户名 / token:<name>
	CtxKeyRole        = "auth_role"     // JWT 角色:admin | viewer(API Token 无角色)
	CtxKeyScope       = "auth_scope"    // 生效 scope:read | write
	CtxKeyAuthKind    = "auth_kind"     // 认证方式:jwt | api_token
	CtxKeyAuditTarget = "audit_target"  // handler 可选设置:审计目标资源标识
)

// bearerPrefix Authorization 头的 Bearer 方案前缀。
const bearerPrefix = "Bearer "
