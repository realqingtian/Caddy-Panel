import { useQuery } from '@tanstack/react-query'
import {
  Alert,
  App,
  Button,
  Card,
  Drawer,
  Input,
  Segmented,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { streamLog, tailLog, type LogKind, type LogTail } from '@/api/logs'
import { useLocaleTag } from '@/hooks/useLocale'
import { apiErrorMessage } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const lineOptions = [100, 200, 500, 1000]

/** 解析后的单行日志(结构化视图数据源)。 */
interface LogRow {
  key: number
  raw: string
  ts: string
  level?: string
  method?: string
  uri?: string
  status?: number
  durationMs?: number
  size?: number
  ip?: string
  msg?: string
}

let keySeq = 0

/** 解析单行:Caddy JSON(access/error)→ 字段化;非 JSON 行回退原始展示。 */
function parseLine(raw: string, localeTag: string): LogRow {
  const row: LogRow = { key: ++keySeq, raw, ts: '' }
  try {
    const j = JSON.parse(raw) as Record<string, unknown>
    // 时间格式跟随面板语言而非浏览器语言(FR-603/I4-D1)
    if (typeof j.ts === 'number') row.ts = new Date(j.ts * 1000).toLocaleString(localeTag)
    else if (typeof j.ts === 'string') row.ts = j.ts
    if (typeof j.level === 'string') row.level = j.level
    if (typeof j.msg === 'string') row.msg = j.msg
    const req = j.request as Record<string, unknown> | undefined
    if (req) {
      if (typeof req.method === 'string') row.method = req.method
      if (typeof req.uri === 'string') row.uri = req.uri
      if (typeof req.remote_ip === 'string') row.ip = req.remote_ip
    }
    if (typeof j.status === 'number') row.status = j.status
    if (typeof j.duration === 'number') row.durationMs = Math.round(j.duration * 1000)
    if (typeof j.size === 'number') row.size = j.size
  } catch {
    // 非 JSON 行:raw 原样回退
  }
  return row
}

const statusTone = (sv?: number): 'success' | 'processing' | 'warning' | 'danger' | undefined => {
  if (!sv) return undefined
  if (sv >= 500) return 'danger'
  if (sv >= 400) return 'warning'
  if (sv >= 300) return 'processing'
  return 'success'
}

/** 日志页:结构化日志视图(解析+筛选+行详情)+原始视图;流式(SSE)默认/轮询回退(I7)。 */
export default function LogsPage() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const token = useAuthStore((s) => s.token)
  const [kind, setKind] = useState<LogKind>('access')
  const [lines, setLines] = useState(200)
  const [keyword, setKeyword] = useState('')
  const [debouncedKw, setDebouncedKw] = useState('')
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [view, setView] = useState<'structured' | 'raw'>('structured')
  const [paused, setPaused] = useState(false)
  // 流式行带筛选键:连接与筛选一一对应,旧连接(筛选变化瞬间的在途批)渲染期自动失效
  const [streamed, setStreamed] = useState<{ key: string; rows: LogRow[] } | null>(null)
  const [detail, setDetail] = useState<LogRow | null>(null)
  const kwTimer = useRef<number | undefined>(undefined)

  const statusKey = statusFilter.join(',')
  const filterKey = `${kind}|${lines}|${debouncedKw}|${statusKey}`
  const tailQuery = useQuery({
    queryKey: ['logtail', kind, lines, debouncedKw, statusKey],
    queryFn: () => tailLog(kind, lines, { keyword: debouncedKw, status: statusKey }),
    refetchOnWindowFocus: false,
  })
  const tail: LogTail | null = tailQuery.data ?? null
  const loadError = tailQuery.error ? apiErrorMessage(tailQuery.error, t('logs.loadFailed')) : ''

  const localeTag = useLocaleTag()
  const parsedTail = useMemo(
    () => (tail?.lines ?? []).map((l) => parseLine(l, localeTag)),
    [tail, localeTag],
  )

  // 流式(SSE,默认):增量行;暂停即关闭连接;筛选变化时由处理器清空增量。
  const closeRef = useRef<(() => void) | undefined>(undefined)
  useEffect(() => {
    if (paused) {
      closeRef.current?.()
      closeRef.current = undefined
      return
    }
    closeRef.current = streamLog(
      kind,
      { keyword: debouncedKw, status: statusKey },
      token ?? '',
      (batch) => {
        setStreamed((prev) => {
          const base = prev && prev.key === filterKey ? prev.rows : []
          const next = base.concat(batch.map((l) => parseLine(l, localeTag)))
          return {
            key: filterKey,
            rows: next.length > 1000 ? next.slice(next.length - 1000) : next,
          }
        })
      },
    )
    return () => closeRef.current?.()
  }, [filterKey, kind, debouncedKw, statusKey, paused, token, localeTag])

  // 结构化视图展示:全量 tail + 流式增量(SSE 只推新行,与全量不重复)。
  const rows = useMemo(() => {
    const liveStreamed = streamed && streamed.key === filterKey ? streamed.rows : []
    if (liveStreamed.length === 0) return parsedTail
    return [...parsedTail, ...liveStreamed].slice(-1000)
  }, [parsedTail, streamed, filterKey])

  const resetView = () => {
    setKeyword('')
    setDebouncedKw('')
    setStatusFilter([])
    setStreamed(null)
  }

  const columns = [
    { title: t('logs.col.time'), dataIndex: 'ts', key: 'ts', width: 170, ellipsis: true },
    ...(kind === 'access'
      ? [
          { title: t('logs.col.method'), dataIndex: 'method', key: 'method', width: 80 },
          { title: t('logs.col.uri'), dataIndex: 'uri', key: 'uri', ellipsis: true },
          {
            title: t('logs.col.status'),
            dataIndex: 'status',
            key: 'status',
            width: 80,
            render: (sv: unknown) => {
              const tone = typeof sv === 'number' ? statusTone(sv) : undefined
              return tone ? <Tag color={tone}>{sv as number}</Tag> : '—'
            },
          },
          {
            title: t('logs.col.duration'),
            dataIndex: 'durationMs',
            key: 'durationMs',
            width: 90,
            render: (v: unknown) => (typeof v === 'number' ? `${v} ms` : '—'),
          },
          {
            title: t('logs.col.size'),
            dataIndex: 'size',
            key: 'size',
            width: 90,
            render: (v: unknown) => (typeof v === 'number' ? v : '—'),
          },
          { title: t('logs.col.ip'), dataIndex: 'ip', key: 'ip', width: 130, ellipsis: true },
        ]
      : [
          { title: t('logs.col.level'), dataIndex: 'level', key: 'level', width: 80 },
          { title: t('logs.col.message'), dataIndex: 'msg', key: 'msg', ellipsis: true },
        ]),
  ]

  return (
    <Card
      title={
        <Space>
          {t('logs.title')}
          <Typography.Text type="secondary" style={{ fontSize: 12, fontWeight: 'normal' }}>
            {t('common.localOnly')}
          </Typography.Text>
        </Space>
      }
      extra={
        <Space wrap>
          <Segmented
            value={kind}
            onChange={(v) => {
              setKind(v as LogKind)
              resetView()
            }}
            options={[
              { value: 'access', label: t('logs.tab.access') },
              { value: 'error', label: t('logs.tab.error') },
            ]}
          />
          <Segmented
            value={view}
            onChange={(v) => setView(v as 'structured' | 'raw')}
            options={[
              { value: 'structured', label: t('logs.view.structured') },
              { value: 'raw', label: t('logs.view.raw') },
            ]}
          />
          <Select
            mode="multiple"
            allowClear
            maxTagCount="responsive"
            placeholder={t('logs.filter.status')}
            style={{ minWidth: 110 }}
            value={statusFilter}
            onChange={(v) => {
              setStatusFilter(v)
              setStreamed(null)
            }}
            options={['2', '3', '4', '5'].map((pfx) => ({ value: pfx, label: `${pfx}xx` }))}
          />
          <Input
            allowClear
            placeholder={t('logs.filter.keyword')}
            style={{ width: 180 }}
            value={keyword}
            onChange={(e) => {
              const v = e.target.value
              setKeyword(v)
              window.clearTimeout(kwTimer.current)
              kwTimer.current = window.setTimeout(() => setDebouncedKw(v), 400)
              setStreamed(null)
            }}
          />
          <Select
            value={lines}
            onChange={(v) => {
              setLines(v)
              setStreamed(null)
            }}
            options={lineOptions.map((n) => ({ value: n, label: `${n} ${t('logs.lines')}` }))}
            style={{ width: 110 }}
          />
          <Space size={4}>
            <Typography.Text type="secondary">{t('logs.autoRefresh')}</Typography.Text>
            <Switch checked={!paused} onChange={(checked) => setPaused(!checked)} />
          </Space>
          <Button loading={tailQuery.isFetching} onClick={() => void tailQuery.refetch()}>
            {t('logs.refreshNow')}
          </Button>
        </Space>
      }
    >
      {loadError && (
        <Alert
          type="error"
          showIcon
          message={t('logs.loadFailed')}
          description={loadError}
          style={{ marginBottom: 12 }}
        />
      )}
      {tail && !tail.exists && (
        <Alert
          type="warning"
          showIcon
          message={t('logs.missing')}
          description={t('logs.missingHint', { path: tail.path })}
          style={{ marginBottom: 12 }}
        />
      )}
      {tail?.truncated && (
        <Alert type="info" showIcon message={t('logs.truncated')} style={{ marginBottom: 12 }} />
      )}

      {view === 'structured' ? (
        <Table<LogRow>
          size="small"
          rowKey="key"
          columns={columns}
          dataSource={rows}
          virtual
          scroll={{ y: 480, x: 900 }}
          pagination={false}
          onRow={(record) => ({ onClick: () => setDetail(record), style: { cursor: 'pointer' } })}
          locale={{ emptyText: t('logs.empty') }}
        />
      ) : (
        <Typography.Paragraph>
          <pre
            className="logs-viewer"
            style={{
              background: 'rgba(128,128,128,0.08)',
              padding: 12,
              borderRadius: 8,
              maxHeight: 560,
              overflow: 'auto',
              fontSize: 12,
              lineHeight: 1.6,
              margin: 0,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-all',
            }}
          >
            {tail?.lines.join('\n') || t('logs.empty')}
          </pre>
        </Typography.Paragraph>
      )}

      <Drawer
        title={t('logs.detailTitle')}
        open={!!detail}
        onClose={() => setDetail(null)}
        width={560}
        extra={
          detail && (
            <Button
              size="small"
              onClick={() => {
                void navigator.clipboard.writeText(detail.raw)
                message.success(t('logs.copyJson'))
              }}
            >
              {t('logs.copyJson')}
            </Button>
          )
        }
      >
        <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontSize: 12 }}>
          {detail?.raw}
        </pre>
      </Drawer>
    </Card>
  )
}
