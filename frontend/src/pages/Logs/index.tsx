import { Alert, Button, Card, Segmented, Select, Space, Switch, Typography } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { tailLog, type LogKind, type LogTail } from '@/api/logs'
import { apiErrorMessage } from '@/api/auth'
import { usePolling } from '@/hooks/usePolling'

const lineOptions = [100, 200, 500, 1000]

/** 日志页:access/error Tab + 自动轮询 tail + 暂停(FR-501/502)。 */
export default function LogsPage() {
  const { t } = useTranslation()
  const [kind, setKind] = useState<LogKind>('access')
  const [lines, setLines] = useState(200)
  const [paused, setPaused] = useState(false)
  const [tail, setTail] = useState<LogTail | null>(null)
  const [loadError, setLoadError] = useState('')

  const { ticking } = usePolling(
    async () => {
      try {
        setTail(await tailLog(kind, lines))
        setLoadError('')
      } catch (error) {
        // 读取失败(常见为日志文件权限)必须可见而非静默显示"暂无内容"(I4-D2)
        setLoadError(apiErrorMessage(error, t('logs.loadFailed')))
      }
    },
    { intervalMs: 3000, paused },
  )

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
        <Space>
          <Segmented
            value={kind}
            onChange={(v) => setKind(v as LogKind)}
            options={[
              { value: 'access', label: t('logs.tab.access') },
              { value: 'error', label: t('logs.tab.error') },
            ]}
          />
          <Select
            value={lines}
            onChange={setLines}
            options={lineOptions.map((n) => ({ value: n, label: `${n} ${t('logs.lines')}` }))}
          />
          <Space size={4}>
            <Typography.Text type="secondary">{t('logs.autoRefresh')}</Typography.Text>
            <Switch checked={!paused} onChange={(checked) => setPaused(!checked)} />
          </Space>
          <Button
            size="small"
            loading={ticking}
            onClick={() => void tailLog(kind, lines).then(setTail)}
          >
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
          {tail?.exists ? tail.lines.join('\n') || t('logs.empty') : t('logs.empty')}
        </pre>
      </Typography.Paragraph>
    </Card>
  )
}
