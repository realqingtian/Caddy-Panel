import { apiClient } from './client'

export interface MetricsPoint {
  ts: string
  value: number
}

export interface MetricsSeries {
  name: string
  labels: Record<string, string>
  points: MetricsPoint[]
}

export interface MetricsSummary {
  window: string
  series: MetricsSeries[]
}

/** 指标时间序列(降采样 60 点;metrics 开关关闭时为空,FR-702)。 */
export async function getMetricsSummary(window: string): Promise<MetricsSummary> {
  const res = await apiClient.get<MetricsSummary>('/metrics/summary', { params: { window } })
  return res.data
}

/** Prometheus 抓取配置示例。 */
export async function getPrometheusConfig(host: string): Promise<string> {
  const res = await apiClient.get<string>('/metrics/prometheus-config', {
    params: { host },
    responseType: 'text',
  })
  return typeof res.data === 'string' ? res.data : String(res.data)
}
