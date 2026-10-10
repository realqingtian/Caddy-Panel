import { apiClient } from './client'

/** 上游实时状态(Caddy GET /reverse_proxy/upstreams 条目,FR-701)。 */
export interface Upstream {
  /** 上游地址,如 127.0.0.1:8080。 */
  dial: string
  /** Caddy 原始状态字段:num_requests / num_fails / fails 等。 */
  [key: string]: unknown
}

/** 读取上游实时状态(透传 Caddy Admin API)。 */
export async function getUpstreams(): Promise<Upstream[]> {
  const res = await apiClient.get<{ upstreams: Upstream[] }>('/upstreams')
  return res.data.upstreams ?? []
}
