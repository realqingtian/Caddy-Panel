import { apiClient } from './client'

export type LogKind = 'access' | 'error'

export interface LogTail {
  path: string
  exists: boolean
  lines: string[]
  truncated: boolean
  keyword?: string
  status?: string
}

export interface LogFilter {
  keyword?: string
  status?: string
}

/** 日志末尾 N 行;文件不存在时 exists=false(FR-501/502);keyword/status 服务端过滤(I7)。 */
export async function tailLog(
  kind: LogKind,
  lines: number,
  filter: LogFilter = {},
): Promise<LogTail> {
  const res = await apiClient.get<LogTail>('/logs', {
    params: {
      type: kind,
      tail: lines,
      keyword: filter.keyword || undefined,
      status: filter.status || undefined,
    },
  })
  return res.data
}

/**
 * 日志实时流(SSE,I7):增量推送新行,onLines 收批;返回关闭函数。
 * token 走查询参数(与终端 WS 同模式)。
 */
export function streamLog(
  kind: LogKind,
  filter: LogFilter,
  token: string,
  onLines: (lines: string[]) => void,
): () => void {
  const es = new EventSource(
    `/api/v1/logs/stream?type=${kind}` +
      `&token=${encodeURIComponent(token)}` +
      (filter.keyword ? `&keyword=${encodeURIComponent(filter.keyword)}` : '') +
      (filter.status ? `&status=${encodeURIComponent(filter.status)}` : ''),
  )
  let buffer: string[] = []
  let timer: number | undefined
  const flush = () => {
    if (buffer.length) {
      const batch = buffer
      buffer = []
      onLines(batch)
    }
  }
  es.addEventListener('line', (ev) => {
    buffer.push((ev as MessageEvent<string>).data)
    if (!timer) timer = window.setTimeout(flush, 200)
  })
  es.onerror = flush
  return () => {
    if (timer) window.clearTimeout(timer)
    flush()
    es.close()
  }
}
