import { apiClient } from './client'

/** 运行状态总览(与后端 service.StatusSummary 对应)。 */
export interface StatusSummary {
  panel_version: string
  caddy_version: string
  caddy_reachable: boolean
  sites_total: number
  sites_enabled: number
  sites_disabled: number
  recent_versions: {
    id: number
    status: 'applied' | 'failed'
    error_msg: string
    created_at: string
  }[]
  drift: {
    in_sync: boolean
    caddy_reachable: boolean
    has_applied: boolean
    db_vs_disk?: { in_sync: boolean; detail: string }
    caddy_vs_disk?: { in_sync: boolean; detail: string }
  }
}

/** 运行状态总览(FR-201/202)。 */
export async function getStatus(): Promise<StatusSummary> {
  const res = await apiClient.get<StatusSummary>('/status')
  return res.data
}
