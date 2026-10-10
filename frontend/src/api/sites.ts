import { apiClient } from './client'

/** 站点领域模型(与后端 model.Site JSON 对应)。 */
export interface Site {
  id: number
  name: string
  type: 'proxy' | 'static' | 'redirect'
  domains: string
  upstreams: string
  lb_policy: string
  websocket: boolean
  headers_enabled: boolean
  custom_headers: string
  root_path: string
  redirect_to: string
  redirect_permanent: boolean
  https_redirect: boolean
  enabled: boolean
  raw_extra: string
  health_check_enabled: boolean
  health_check_path: string
  health_check_interval: string
  health_check_expected: number
  created_at: string
  updated_at: string
}

/** 站点创建/更新输入(与后端 service.SiteInput 对应)。 */
export interface SiteInput {
  name: string
  type: Site['type']
  domains: string
  upstreams: string
  lb_policy: string
  websocket: boolean
  headers_enabled: boolean
  custom_headers: string
  root_path: string
  redirect_to: string
  redirect_permanent: boolean
  https_redirect: boolean
  enabled: boolean
  health_check_enabled: boolean
  health_check_path: string
  health_check_interval: string
  health_check_expected: number
}

export interface SiteMutationResponse {
  site: Site
  applied: boolean
}

/** 站点列表。 */
export async function listSites(): Promise<Site[]> {
  const res = await apiClient.get<{ sites: Site[] }>('/sites')
  return res.data.sites
}

/** 站点详情。 */
export async function getSite(id: number): Promise<Site> {
  const res = await apiClient.get<Site>(`/sites/${id}`)
  return res.data
}

/** 创建站点;dryRun 仅预检(后端校验渲染,不落库不生效)。 */
export async function createSite(input: SiteInput, dryRun = false): Promise<SiteMutationResponse> {
  const res = await apiClient.post<SiteMutationResponse>('/sites', input, {
    params: dryRun ? { dry_run: true } : undefined,
  })
  return res.data
}

/** 更新站点。 */
export async function updateSite(id: number, input: SiteInput): Promise<SiteMutationResponse> {
  const res = await apiClient.put<SiteMutationResponse>(`/sites/${id}`, input)
  return res.data
}

/** 删除站点(下发光含该站的配置后删除记录)。 */
export async function deleteSite(id: number): Promise<void> {
  await apiClient.delete(`/sites/${id}`)
}

/** 启停站点。 */
export async function setSiteEnabled(id: number, enabled: boolean): Promise<SiteMutationResponse> {
  const res = await apiClient.post<SiteMutationResponse>(
    `/sites/${id}/${enabled ? 'enable' : 'disable'}`,
  )
  return res.data
}

/** 单域名证书状态(FR-308)。 */
export interface DomainCertStatus {
  domain: string
  found: boolean
  subject?: string
  not_after?: string
  not_before?: string
}

export interface SiteCertStatus {
  site_id: number
  name: string
  domains: DomainCertStatus[]
  any_found: boolean
}

/** 站点证书签发状态(只读;证书管理属 M2)。 */
export async function getSiteCertificate(id: number): Promise<SiteCertStatus> {
  const res = await apiClient.get<SiteCertStatus>(`/sites/${id}/certificate`)
  return res.data
}
