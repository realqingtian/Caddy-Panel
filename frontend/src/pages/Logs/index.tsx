import { Alert, Button, Card, Segmented, Select, Space, Switch, Typography } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { tailLog, type LogKind, type LogTail } from '@/api/logs'
import { usePolling } from '@/hooks/usePolling'

const lineOptions = [100, 200, 500, 1000]

/** 日志页:access/error Tab + 自动轮询 tail + 暂停(FR-501/502)。 */
export default function LogsPage() {
  const { t } = useTranslation()
  const [kind, setKind] = useState<LogKind>('access')
  const [lines, setLines] = useState(200)
  const [paused, setPaused] = useState(false)
  const [tail, setTail] = useState<LogTail | null>(null)

  const { ticking } = usePolling(
    async () => {
      setTail(await tailLog(kind, lines))
    },
    { intervalMs: 3000, paused },
  )

  return (
    <Card
      title={t('logs.title')}
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
