import { apiClient } from './client'

export interface ApiToken {
  id: number
  name: string
  prefix: string
  scopes: 'read' | 'write'
  last_used_at: string | null
  expires_at: string | null
  created_at: string
}

export interface CreateTokenResult {
  plain_token: string
  token: ApiToken
}

/** Token 列表(不含明文)。 */
export async function listTokens(): Promise<ApiToken[]> {
  const res = await apiClient.get<ApiToken[]>('/tokens')
  return res.data
}

/** 签发 token(明文仅本次返回)。 */
export async function createToken(
  name: string,
  scope: string,
  expiresIn: string,
): Promise<CreateTokenResult> {
  const res = await apiClient.post<CreateTokenResult>('/tokens', {
    name,
    scope,
    expires_in: expiresIn || undefined,
  })
  return res.data
}

/** 删除 token。 */
export async function deleteToken(id: number): Promise<void> {
  await apiClient.delete(`/tokens/${id}`)
}
