import { useQuery } from '@tanstack/react-query'
import {
  CodeOutlined,
  GlobalOutlined,
  MonitorOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons'
import { Alert, Card, Col, List, Row, Space, Statistic, Tag, Typography, theme } from 'antd'
import { useTranslation } from 'react-i18next'

import { getStatus } from '@/api/dashboard'
import type { ReactNode } from 'react'

/** Dashboard:运行状态卡片 + 漂移告警 + 最近下发(FR-201/202)。 */
export default function DashboardPage() {
  const { t } = useTranslation()
  const { token } = theme.useToken()
  const statusQuery = useQuery({ queryKey: ['status'], queryFn: getStatus, refetchInterval: 10000 })
  const status = statusQuery.data

  const caddyOk = status?.caddy_reachable ?? false
  const driftOk = status?.drift?.in_sync ?? false

  // 统计卡语义图标:图标与数值同色(成功/警告/错误/主色),全部取 antd 预设 token,亮暗自适应。
  const statCards: {
    key: string
    icon: ReactNode
    tone: string
    bg: string
    value: string | number
    valueColor?: string
    footer?: ReactNode
  }[] = [
    {
      key: 'caddy',
      icon: <MonitorOutlined />,
      tone: caddyOk ? token.colorSuccess : token.colorError,
      bg: caddyOk ? token.colorSuccessBg : token.colorErrorBg,
      value: caddyOk ? t('dashboard.caddyUp') : t('dashboard.caddyDown'),
      valueColor: caddyOk ? token.colorSuccess : token.colorError,
      footer: status?.caddy_version && (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {status.caddy_version}
        </Typography.Text>
      ),
    },
    {
      key: 'panel',
      icon: <CodeOutlined />,
      tone: token.colorTextSecondary,
      bg: token.colorFillSecondary,
      value: status?.panel_version ?? '…',
    },
    {
      key: 'sites',
      icon: <GlobalOutlined />,
      tone: token.colorPrimary,
      bg: token.colorPrimaryBg,
      value: status?.sites_total ?? 0,
      footer: (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {t('dashboard.sitesBreakdown', {
            enabled: status?.sites_enabled ?? 0,
            disabled: status?.sites_disabled ?? 0,
          })}
        </Typography.Text>
      ),
    },
    {
      key: 'drift',
      icon: <SafetyCertificateOutlined />,
      // Caddy 不可达时一致性不可知("—"),用中性色;仅在可达时给出 成功/警告 语义色
      tone: caddyOk
        ? driftOk
          ? token.colorSuccess
          : token.colorWarning
        : token.colorTextSecondary,
      bg: caddyOk
        ? driftOk
          ? token.colorSuccessBg
          : token.colorWarningBg
        : token.colorFillSecondary,
      value: status
        ? caddyOk
          ? driftOk
            ? t('dashboard.driftOk')
            : t('dashboard.driftDetected')
          : '—'
        : '…',
      valueColor: caddyOk
        ? driftOk
          ? token.colorSuccess
          : token.colorWarning
        : token.colorTextSecondary,
    },
  ]

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      {status && !status.caddy_reachable && (
        <Alert type="error" showIcon message={t('dashboard.caddyUnreachable')} />
      )}
      {status?.drift && status.drift.caddy_reachable && !status.drift.in_sync && (
        <Alert
          type="warning"
          showIcon
          message={t('dashboard.driftTitle')}
          description={
            <ul style={{ margin: 0, paddingLeft: 18 }}>
              {!status.drift.in_sync &&
                status.drift.db_vs_disk &&
                !status.drift.db_vs_disk.in_sync && (
                  <li>
                    {t('dashboard.drift.dbVsDisk')}: {status.drift.db_vs_disk.detail}
                  </li>
                )}
              {status.drift.caddy_vs_disk && !status.drift.caddy_vs_disk.in_sync && (
                <li>
                  {t('dashboard.drift.caddyVsDisk')}: {status.drift.caddy_vs_disk.detail}
                </li>
              )}
            </ul>
          }
        />
      )}

      <Row gutter={16}>
        {statCards.map((card) => (
          <Col span={6} key={card.key}>
            {/* 高度 100%:无论有无副文本,四卡在 Row 内等高 */}
            <Card style={{ height: '100%' }}>
              <div className="stat-head">
                <span className="stat-icon" style={{ background: card.bg, color: card.tone }}>
                  {card.icon}
                </span>
                <Typography.Text type="secondary">
                  {t(`dashboard.stat.${card.key}`)}
                </Typography.Text>
              </div>
              <Statistic value={card.value} valueStyle={{ color: card.valueColor, fontSize: 22 }} />
              {card.footer}
            </Card>
          </Col>
        ))}
      </Row>

      {status && status.nodes.length > 0 && (
        <Card title={t('dashboard.nodesTitle')}>
          <Row gutter={16}>
            {status.nodes.map((node) => (
              <Col span={6} key={node.node_id} style={{ marginBottom: 12 }}>
                <Card size="small" title={node.name}>
                  <Space size={4} wrap>
                    <Tag color={node.status === 'online' ? 'success' : 'default'}>
                      {t('nodes.status.' + node.status)}
                    </Tag>
                    <Tag color={node.caddy_ok ? 'success' : 'error'}>{t('nodes.caddy')}</Tag>
                    <Tag color={node.in_sync ? 'success' : 'warning'}>{t('nodes.inSync')}</Tag>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {t('nodes.field.sites')}: {node.site_count}
                    </Typography.Text>
                  </Space>
                </Card>
              </Col>
            ))}
          </Row>
        </Card>
      )}

      <Card title={t('dashboard.recentTitle')}>
        <List
          size="small"
          dataSource={status?.recent_versions ?? []}
          locale={{ emptyText: t('dashboard.noVersions') }}
          renderItem={(version) => (
            <List.Item>
              <Space>
                <Typography.Text type="secondary">#{version.id}</Typography.Text>
                {version.status === 'applied' ? (
                  <Tag color="green">{t('config.status.applied')}</Tag>
                ) : (
                  <Tag color="red">{t('config.status.failed')}</Tag>
                )}
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {new Date(version.created_at).toLocaleString()}
                </Typography.Text>
                {version.error_msg && (
                  <Typography.Text type="danger" style={{ fontSize: 12 }}>
                    {version.error_msg}
                  </Typography.Text>
                )}
              </Space>
            </List.Item>
          )}
        />
      </Card>
    </Space>
  )
}
