import { useQuery } from '@tanstack/react-query'
import { Alert, App, Button, Card, Segmented, Select, Skeleton, Space, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { apiErrorMessage } from '@/api/auth'
import { listNodes } from '@/api/nodes'
import { getMetricsSummary, getPrometheusConfig, type MetricsSeries } from '@/api/metrics'

const windows = ['1h', '6h', '24h']

// 稳定的序列颜色(chart 数据 key 需要)。
const palette = ['#1677ff', '#52c41a', '#faad14', '#eb2f96', '#722ed1', '#13c2c2']

/** 监控页:抓取样本折线图 + Prometheus 配置生成(FR-702)。 */
export default function MetricsPage() {
  const { t } = useTranslation()
  const { message, modal } = App.useApp()
  const [window, setWindow] = useState('1h')
  const [metric, setMetric] = useState('caddy_http_requests_total')
  const [nodeId, setNodeId] = useState('local')

  // 节点选择(local + 已接入节点;m3-design §3.2 第 6 条按节点维度展示)。
  const nodesQuery = useQuery({ queryKey: ['nodes'], queryFn: listNodes })
  const nodeOptions = [
    { value: 'local', label: t('sites.node.local') },
    ...(nodesQuery.data ?? []).map((n) => ({ value: n.node_id, label: n.name })),
  ]

  const summaryQuery = useQuery({
    queryKey: ['metrics', window, nodeId],
    queryFn: () => getMetricsSummary(window, nodeId),
    refetchInterval: 15000,
  })

  const metricNames = useMemo(() => {
    const names = new Set((summaryQuery.data?.series ?? []).map((s) => s.name))
    return [...names].sort()
  }, [summaryQuery.data])

  const series = useMemo(
    () => (summaryQuery.data?.series ?? []).filter((s) => s.name === metric),
    [summaryQuery.data, metric],
  )

  // 以并集时间轴组织 chart 数据:每个时间点一行,各序列一列。
  const chartData = useMemo(() => {
    const tsMap = new Map<string, Record<string, number>>()
    series.forEach((s: MetricsSeries) => {
      const key = s.labels.host || s.labels.instance || 'total'
      s.points.forEach((p) => {
        const row = tsMap.get(p.ts) ?? {}
        row[key] = p.value
        tsMap.set(p.ts, row)
      })
    })
    return [...tsMap.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([ts, row]) => ({ ts: new Date(ts).toLocaleTimeString(), ...row }))
  }, [series])

  const seriesKeys = useMemo(() => {
    const keys = new Set<string>()
    chartData.forEach((row) =>
      Object.keys(row).forEach((k) => {
        if (k !== 'ts') keys.add(k)
      }),
    )
    return [...keys].sort()
  }, [chartData])

  const showPromConfig = async () => {
    try {
      const config = await getPrometheusConfig('caddy:80')
      modal.info({
        title: t('metrics.promConfigTitle'),
        width: 640,
        content: <pre style={{ whiteSpace: 'pre-wrap', fontSize: 12 }}>{config}</pre>,
      })
    } catch (error) {
      message.error(apiErrorMessage(error, t('metrics.loadFailed')))
    }
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card
        title={t('metrics.title')}
        extra={
          <Space>
            <Select
              value={nodeId}
              onChange={setNodeId}
              style={{ minWidth: 160 }}
              options={nodeOptions}
            />
            <Segmented value={window} onChange={(v) => setWindow(v as string)} options={windows} />
            <Select
              value={metric}
              onChange={setMetric}
              style={{ minWidth: 280 }}
              options={metricNames.map((name) => ({ value: name, label: name }))}
              showSearch
            />
            <Button onClick={() => void showPromConfig()}>{t('metrics.promConfig')}</Button>
          </Space>
        }
      >
        {summaryQuery.isPending ? (
          <Skeleton active title={false} paragraph={{ rows: 6 }} />
        ) : (
          <>
            {metricNames.length === 0 && (
              <Alert
                type="info"
                showIcon
                message={t('metrics.emptyTitle')}
                description={t('metrics.emptyHint')}
              />
            )}
            {metricNames.length > 0 && series.length === 0 && (
              <Alert type="info" showIcon message={t('metrics.noSeries')} />
            )}
            {chartData.length > 0 && (
              <ResponsiveContainer width="100%" height={480}>
                <LineChart data={chartData} margin={{ top: 8, right: 24, bottom: 8, left: 8 }}>
                  <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
                  <XAxis dataKey="ts" fontSize={12} />
                  <YAxis fontSize={12} />
                  <Tooltip contentStyle={{ fontSize: 12 }} />
                  <Legend wrapperStyle={{ fontSize: 12 }} />
                  {seriesKeys.map((key, index) => (
                    <Line
                      key={key}
                      type="monotone"
                      dataKey={key}
                      stroke={palette[index % palette.length]}
                      dot={false}
                      strokeWidth={2}
                    />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            )}
          </>
        )}
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {t('metrics.counterHint')}
        </Typography.Text>
      </Card>
    </Space>
  )
}
