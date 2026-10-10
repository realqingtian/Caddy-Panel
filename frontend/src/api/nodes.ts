import { apiClient } from './client'

/** 远程节点(与后端 model.Node JSON 对应;local 为内置不入列表)。 */
export interface Node {
  id: number
  node_id: string
  name: string
  secret_prefix: string
  status: 'online' | 'offline'
  agent_version: string
  platform: string
  caddy_version: string
  admin_bind: string
  caddyfile_path: string
  caddy_ok: boolean
  in_sync: boolean
  site_count: number
  last_seen_at: string | null
  enrolled_at: string
}

/** 接入令牌记录(仅前缀,无明文)。 */
export interface EnrollmentToken {
  id: number
  node_name: string
  platform: string
  prefix: string
  expires_at: string
  used_at: string | null
  revoked: boolean
  created_at: string
}

export interface CreateEnrollTokenResult {
  plain_token: string
  install_command: string
  token: EnrollmentToken
}

/** 节点列表。 */
export async function listNodes(): Promise<Node[]> {
  const res = await apiClient.get<Node[]>('/nodes')
  return res.data
}

/** 删除节点(名下有站点时后端 409)。 */
export async function deleteNode(nodeId: string): Promise<void> {
  await apiClient.delete(`/nodes/${nodeId}`)
}

/** 接入令牌列表。 */
export async function listEnrollTokens(): Promise<EnrollmentToken[]> {
  const res = await apiClient.get<EnrollmentToken[]>('/nodes/enrollment-tokens')
  return res.data
}

/** 签发接入令牌 + 安装命令(明文仅本次返回)。 */
export async function createEnrollToken(
  nodeName: string,
  platform: string,
): Promise<CreateEnrollTokenResult> {
  const res = await apiClient.post<CreateEnrollTokenResult>('/nodes/enrollment-tokens', {
    node_name: nodeName,
    platform,
  })
  return res.data
}

/** 撤销接入令牌。 */
export async function revokeEnrollToken(id: number): Promise<void> {
  await apiClient.post(`/nodes/enrollment-tokens/${id}/revoke`)
}

/** 删除接入令牌记录。 */
export async function deleteEnrollToken(id: number): Promise<void> {
  await apiClient.delete(`/nodes/enrollment-tokens/${id}`)
}

/** 卸载命令(对称生成)。 */
export async function getUninstallCommand(): Promise<string> {
  const res = await apiClient.get<{ command: string }>('/nodes/uninstall-command')
  return res.data.command
}
