import { apiClient } from './client'

/** 配置版本(与后端 model.ConfigVersion 对应)。 */
export interface ConfigVersion {
  id: number
  sites_snapshot: string
  rendered_caddyfile: string
  status: 'applied' | 'failed'
  error_msg: string
  created_at: string
}

/** 系统设置(键 → 生效值,FR-601)。 */
export type Settings = Record<string, string>

/** 版本历史(新→旧)。 */
export async function listVersions(): Promise<ConfigVersion[]> {
  const res = await apiClient.get<{ versions: ConfigVersion[] }>('/config/versions')
  return res.data.versions
}

/** 版本详情(含 Caddyfile 全文)。 */
export async function getVersion(id: number): Promise<ConfigVersion> {
  const res = await apiClient.get<ConfigVersion>(`/config/versions/${id}`)
  return res.data
}

/** 回滚到指定版本。 */
export async function rollbackTo(id: number): Promise<ConfigVersion> {
  const res = await apiClient.post<ConfigVersion>(`/config/versions/${id}/rollback`)
  return res.data
}

/** 读取磁盘上的渲染产物。 */
export async function getCurrentCaddyfile(): Promise<string> {
  const res = await apiClient.get<{ content: string }>('/config/caddyfile')
  return res.data.content
}

/** 直接编辑 Caddyfile:校验通过才生效。 */
export async function saveCaddyfile(content: string): Promise<ConfigVersion> {
  const res = await apiClient.post<ConfigVersion>('/config/caddyfile', { content })
  return res.data
}

/** Caddy 当前生效配置 JSON(GET /config 透传)。 */
export async function getCurrentJSON(): Promise<string> {
  const res = await apiClient.get<string>('/config/current', { responseType: 'text' })
  return typeof res.data === 'string' ? res.data : JSON.stringify(res.data)
}

/** 读取系统设置。 */
export async function getSettings(): Promise<Settings> {
  const res = await apiClient.get<{ settings: Settings }>('/settings')
  return res.data.settings
}

/** 更新系统设置。 */
export async function updateSettings(settings: Settings): Promise<Settings> {
  const res = await apiClient.put<{ settings: Settings }>('/settings', { settings })
  return res.data.settings
}
