import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Col, List, Row, Space, Statistic, Tag, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import { getStatus } from '@/api/dashboard'

/** Dashboard:运行状态卡片 + 漂移告警 + 最近下发(FR-201/202)。 */
export default function DashboardPage() {
  const { t } = useTranslation()
  const statusQuery = useQuery({ queryKey: ['status'], queryFn: getStatus, refetchInterval: 10000 })
  const status = statusQuery.data

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
        <Col span={6}>
          <Card>
            <Statistic
              title={t('dashboard.stat.caddy')}
              value={status?.caddy_reachable ? t('dashboard.caddyUp') : t('dashboard.caddyDown')}
              valueStyle={{ color: status?.caddy_reachable ? '#3f8600' : '#cf1322', fontSize: 22 }}
            />
            {status?.caddy_version && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {status.caddy_version}
              </Typography.Text>
            )}
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title={t('dashboard.stat.panel')} value={status?.panel_version ?? '…'} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title={t('dashboard.stat.sites')} value={status?.sites_total ?? 0} />
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('dashboard.sitesBreakdown', {
                enabled: status?.sites_enabled ?? 0,
                disabled: status?.sites_disabled ?? 0,
              })}
            </Typography.Text>
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title={t('dashboard.stat.drift')}
              value={
                status
                  ? status.caddy_reachable
                    ? status.drift?.in_sync
                      ? t('dashboard.driftOk')
                      : t('dashboard.driftDetected')
                    : '—'
                  : '…'
              }
              valueStyle={{
                color: status?.drift?.in_sync ? '#3f8600' : '#d46b08',
                fontSize: 22,
              }}
            />
          </Card>
        </Col>
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
