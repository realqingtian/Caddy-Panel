import { Card, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

export type PlaceholderKey = 'dashboard' | 'sites' | 'config' | 'logs' | 'settings'

interface PlaceholderPageProps {
  page: PlaceholderKey
}

/** M1 各页面骨架占位:仅展示标题与实现节点说明,功能随后续任务落地。 */
export function PlaceholderPage({ page }: PlaceholderPageProps) {
  const { t } = useTranslation()
  return (
    <Card title={t(`pages.${page}.title`)}>
      <Typography.Text type="secondary">{t(`pages.${page}.placeholder`)}</Typography.Text>
    </Card>
  )
}
