import { useAuthStore } from '@/stores/auth'

/**
 * 权限点常量:与后端 model/permission.go 保持同构(修改须双向同步)。
 * 设计见 docs/m4-users-design.md §3。
 */
export const Perm = {
  SitesRead: 'sites:read',
  SitesWrite: 'sites:write',
  NodesRead: 'nodes:read',
  NodesWrite: 'nodes:write',
  ConfigRead: 'config:read',
  ConfigRollback: 'config:rollback',
  ConfigDirectEdit: 'config:direct-edit',
  LogsRead: 'logs:read',
  MetricsRead: 'metrics:read',
  UpstreamsRead: 'upstreams:read',
  SettingsRead: 'settings:read',
  SettingsWrite: 'settings:write',
  TokensManage: 'tokens:manage',
  BackupExport: 'backup:export',
  BackupRestore: 'backup:restore',
  TerminalUse: 'terminal:use',
  UsersManage: 'users:manage',
} as const

/** 全部权限点(audit:read 无前端消费,不入矩阵)。 */
const ALL: string[] = Object.values(Perm)

/** editor 禁持清单:高危三点 + 设置写/恢复/Token 管理。 */
const EDITOR_DENIED: string[] = [
  Perm.UsersManage,
  Perm.TerminalUse,
  Perm.ConfigDirectEdit,
  Perm.SettingsWrite,
  Perm.BackupRestore,
  Perm.TokensManage,
]

/** 角色 → 权限集(与后端 RolePermissions 同构;admin 通配)。 */
function permissionsOf(role: string | null): string[] {
  switch (role) {
    case 'admin':
      return ['*']
    case 'editor':
      return ALL.filter((p) => !EDITOR_DENIED.includes(p))
    case 'viewer':
      // settings:read 不派发(设置页整页不可见,§3.4)
      return ALL.filter((p) => p.endsWith(':read') && p !== Perm.SettingsRead)
    default:
      return []
  }
}

/** 权限判断:单个权限点。 */
export function hasPermission(role: string | null, required: string): boolean {
  return permissionsOf(role).some((p) => p === '*' || p === required)
}

/** usePermission:当前登录角色的权限判断 hook。 */
export function usePermission() {
  const role = useAuthStore((state) => state.role)
  return {
    can: (required: string) => hasPermission(role, required),
    /** viewer 只读角色的便捷判断(整页只读场景)。 */
    readOnly: role === 'viewer',
  }
}
