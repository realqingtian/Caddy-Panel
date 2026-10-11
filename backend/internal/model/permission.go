// Package model — 权限点常量(权限字符串 = <资源>:<操作>)。
// 设计与角色矩阵见 docs/m4-users-design.md §3;**仅新增权限点常量,禁止在校验外旁路**。
package model

import "strings"

// 权限点常量。
const (
	PermSitesRead      = "sites:read"
	PermSitesWrite     = "sites:write"
	PermNodesRead      = "nodes:read"
	PermNodesWrite     = "nodes:write"
	PermConfigRead     = "config:read"
	PermConfigRollback = "config:rollback"
	PermLogsRead       = "logs:read"
	PermMetricsRead    = "metrics:read"
	PermUpstreamsRead  = "upstreams:read"
	PermSettingsRead   = "settings:read"
	PermSettingsWrite  = "settings:write"
	PermTokensManage   = "tokens:manage"
	PermBackupExport   = "backup:export"
	PermBackupRestore  = "backup:restore"
	PermAuditRead      = "audit:read" // 预留:M4 无查看接口,不入任何角色矩阵
	PermTerminalUse    = "terminal:use"
	PermUsersManage    = "users:manage"
	// PermConfigDirectEdit 直接编辑 Caddyfile:可改全局块(含 admin 绑定),高危,仅 admin。
	PermConfigDirectEdit = "config:direct-edit"
)

// 高危权限点:任何非 admin 主体(API Token / editor / viewer)一律不得持有;
// 见设计 §3.2 硬规则。RolePermissions 与 Token 权限集构造均引用此清单。
var HighRiskPerms = []string{
	PermUsersManage,
	PermTerminalUse,
	PermConfigDirectEdit,
}

// 高危权限点集合(便于 O(1) 判断)。
var highRiskSet = func() map[string]bool {
	set := make(map[string]bool, len(HighRiskPerms))
	for _, p := range HighRiskPerms {
		set[p] = true
	}
	return set
}()

// IsHighRisk 是否高危权限点。
func IsHighRisk(perm string) bool { return highRiskSet[perm] }

// activePerms 参与角色矩阵的权限点(audit:read 本期无消费接口,不入任何角色,见设计 §3.1)。
var activePerms = []string{
	PermSitesRead, PermSitesWrite,
	PermNodesRead, PermNodesWrite,
	PermConfigRead, PermConfigRollback, PermConfigDirectEdit,
	PermLogsRead, PermMetricsRead, PermUpstreamsRead,
	PermSettingsRead, PermSettingsWrite,
	PermTokensManage,
	PermBackupExport, PermBackupRestore,
	PermTerminalUse,
	PermUsersManage,
}

// editor 禁持清单:高危三点 + 设置写/恢复/Token 管理。
var editorDenied = map[string]bool{
	PermUsersManage:    true,
	PermTerminalUse:    true,
	PermConfigDirectEdit: true,
	PermSettingsWrite:  true,
	PermBackupRestore:  true,
	PermTokensManage:   true,
}

// RolePermissions 角色 → 权限点集合;admin 为通配 "*"。
// 未知角色返回空集合(无权限);历史空 role 由中间件归一为 admin 后再调用。
func RolePermissions(role string) []string {
	switch role {
	case RoleAdmin:
		return []string{"*"}
	case RoleEditor:
		out := make([]string, 0, len(activePerms))
		for _, p := range activePerms {
			if !editorDenied[p] {
				out = append(out, p)
			}
		}
		return out
	case RoleViewer:
		// settings:read 不派发给 viewer(设置页整页不可见,§3.4;
		// 设置项含 SMTP/路径等运维敏感信息);audit:read 本就未入矩阵。
		out := make([]string, 0, len(activePerms))
		for _, p := range activePerms {
			if strings.HasSuffix(p, ":read") && p != PermSettingsRead {
				out = append(out, p)
			}
		}
		return out
	default:
		return nil
	}
}

// TokenPermissions API Token scope → 权限点集合:
// read = 全部只读点;write = 全部读写点(高危三点与 audit:read 恒不含,§3.3)。
func TokenPermissions(scopes []string) []string {
	set := map[string]bool{}
	for _, scope := range scopes {
		for _, p := range activePerms {
			if strings.HasSuffix(p, ":read") {
				set[p] = true
				continue
			}
			if scope == ScopeWrite && strings.HasSuffix(p, ":write") && !IsHighRisk(p) {
				set[p] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out
}

// HasPermission 权限集是否满足要求:"*" 通配全部。
func HasPermission(perms []string, required string) bool {
	for _, p := range perms {
		if p == "*" || p == required {
			return true
		}
	}
	return false
}
