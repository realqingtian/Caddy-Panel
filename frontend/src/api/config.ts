import { apiClient } from './client'

/** 配置版本(与后端 model.ConfigVersion 对应)。 */
export interface ConfigVersion {
  id: number
  sites_snapshot: string
  snippets_snapshot?: string
  rendered_caddyfile: string
  status: 'applied' | 'failed'
  error_msg: string
  notes: string
  created_at: string
}

/** Caddyfile 附加片段(与后端 model.Snippet 对应)。 */
export interface Snippet {
  id: number
  name: string
  content: string
  enabled: boolean
  created_at: string
  updated_at: string
}

/** 片段新建/编辑输入。 */
export interface SnippetInput {
  name: string
  content: string
  enabled: boolean
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

/** 编辑版本备注(空字符串清空;仅 admin)。 */
export async function updateVersionNotes(id: number, notes: string): Promise<ConfigVersion> {
  const res = await apiClient.patch<ConfigVersion>(`/config/versions/${id}/notes`, { notes })
  return res.data
}

/** 片段列表(ID 升序 = 渲染追加顺序)。 */
export async function listSnippets(): Promise<Snippet[]> {
  const res = await apiClient.get<{ snippets: Snippet[] }>('/config/snippets')
  return res.data.snippets
}

/** 新建片段:校验通过并下发成功才落库。 */
export async function createSnippet(input: SnippetInput): Promise<Snippet> {
  const res = await apiClient.post<Snippet>('/config/snippets', input)
  return res.data
}

/** 编辑片段(名称/内容/启用开关),变更经管线下发。 */
export async function updateSnippet(id: number, input: SnippetInput): Promise<Snippet> {
  const res = await apiClient.put<Snippet>(`/config/snippets/${id}`, input)
  return res.data
}

/** 删除片段(启用片段先重下发移除产物块)。 */
export async function deleteSnippet(id: number): Promise<void> {
  await apiClient.delete(`/config/snippets/${id}`)
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
