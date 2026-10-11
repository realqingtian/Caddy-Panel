import { useQuery } from '@tanstack/react-query'
import { Card, Space, Table, Typography, type TableProps } from 'antd'
import { useTranslation } from 'react-i18next'

import { tablePagination } from '@/lib/tablePagination'

import { getUpstreams, type Upstream } from '@/api/upstreams'

interface UpstreamsTableProps {
  /** 轮询间隔毫秒;0 表示不轮询。 */
  refreshInterval?: number
  /** 不带卡片外框的紧凑模式。 */
  compact?: boolean
}

/** 上游状态表:地址/总请求/失败数(FR-701)。 */
export function UpstreamsTable({
  refreshInterval = 5000,
  compact = false,
}: UpstreamsTableProps = {}) {
  const { t } = useTranslation()
  const upstreamsQuery = useQuery({
    queryKey: ['upstreams'],
    queryFn: getUpstreams,
    refetchInterval: refreshInterval || false,
    retry: 0,
  })

  const columns: TableProps<Upstream>['columns'] = [
    { title: t('upstreams.column.dial'), dataIndex: 'dial', key: 'dial' },
    {
      title: t('upstreams.column.requests'),
      key: 'requests',
      render: (_: unknown, record: Upstream) => Number(record.num_requests ?? 0),
    },
    {
      title: t('upstreams.column.fails'),
      key: 'fails',
      render: (_: unknown, record: Upstream) => {
        const fails = Number(record.num_fails ?? 0)
        return <span style={{ color: fails > 0 ? '#cf1322' : undefined }}>{fails}</span>
      },
    },
  ]

  const table = (
    <Table<Upstream>
      pagination={tablePagination(t)}
      rowKey="dial"
      size="small"
      loading={upstreamsQuery.isLoading}
      dataSource={upstreamsQuery.data ?? []}
      columns={columns}
      locale={{ emptyText: t('upstreams.empty') }}
    />
  )
  if (compact) return table
  return (
    <Card
      title={
        <Space>
          {t('upstreams.title')}
          <Typography.Text type="secondary" style={{ fontSize: 12, fontWeight: 'normal' }}>
            {t('common.localOnly')}
          </Typography.Text>
        </Space>
      }
    >
      {table}
    </Card>
  )
}
