// Package middleware 提供 Gin 中间件:请求语言解析、JWT 鉴权、审计写入。
// 登录失败锁定为业务规则,实现在 service.LoginGuard,不走中间件。
package middleware

// context 键:中间件写入、handler 读取。
const (
	CtxKeyLang        = "lang"          // i18n 解析后的语言(如 zh-CN)
	CtxKeyUserID      = "auth_user_id"  // JWT 中间件写入的用户 ID
	CtxKeyUsername    = "auth_username" // JWT 中间件写入的用户名
	CtxKeyAuditTarget = "audit_target"  // handler 可选设置:审计目标资源标识
)
