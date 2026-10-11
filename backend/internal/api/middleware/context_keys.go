// Package middleware 提供 Gin 中间件:请求语言解析、双轨认证(JWT/API Token)、
// 权限点校验、强制改密闸门与审计写入。
// 登录失败锁定为业务规则,实现在 service.LoginGuard,不走中间件。
package middleware

// context 键:中间件写入、handler 读取。
const (
	CtxKeyLang         = "lang"          // i18n 解析后的语言(如 zh-CN)
	CtxKeyUserID       = "auth_user_id"  // JWT 用户 ID;API Token 主体为 0(审计记为空)
	CtxKeyUsername     = "auth_username" // JWT 用户名 / token:<name>
	CtxKeyRole         = "auth_role"     // JWT 角色:admin | editor | viewer(API Token 无角色)
	CtxKeyScope        = "auth_scope"    // M3 二值 scope(read|write):遗留读取点兼容,新代码用 Permissions
	CtxKeyAuthKind     = "auth_kind"     // 认证方式:jwt | api_token
	CtxKeySID          = "auth_sid"      // JWT 会话 ID(M4 可吊销会话)
	CtxKeyPermissions  = "auth_permissions" // 权限点集合([]string;admin 为 ["*"])
	CtxKeyMustChangePass = "auth_must_change_password" // 是否待强制改密(bool)
	CtxKeyAuditTarget  = "audit_target"  // handler 可选设置:审计目标资源标识
	CtxKeyAuditNodeID  = "audit_node_id" // handler 可选设置:审计目标资源所属节点(空=default local)
)

// bearerPrefix Authorization 头的 Bearer 方案前缀。
const bearerPrefix = "Bearer "
