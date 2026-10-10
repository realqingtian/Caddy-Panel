import { apiClient } from './client'

export type LogKind = 'access' | 'error'

export interface LogTail {
  path: string
  exists: boolean
  lines: string[]
  truncated: boolean
}

/** 日志末尾 N 行;文件不存在时 exists=false(FR-501/502)。 */
export async function tailLog(kind: LogKind, lines: number): Promise<LogTail> {
  const res = await apiClient.get<LogTail>('/logs', { params: { type: kind, tail: lines } })
  return res.data
}
