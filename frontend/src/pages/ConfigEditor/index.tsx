import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Card, Col, Popconfirm, Row, Segmented, Space, Tag, Typography } from 'antd'
import { DiffEditor } from '@monaco-editor/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import {
  getCurrentCaddyfile,
  listVersions,
  rollbackTo,
  saveCaddyfile,
  type ConfigVersion,
} from '@/api/config'
import { CaddyfileEditor } from '@/components/CaddyfileEditor'

type EditorMode = 'preview' | 'edit'

/** 配置中心:渲染产物预览/直编 + 版本历史 + diff + 回滚(FR-401~404)。 */
export default function ConfigEditorPage() {
  const { t } = useTranslation()
  const { message, notification } = App.useApp()
  const queryClient = useQueryClient()

  const [mode, setMode] = useState<EditorMode>('preview')
  const [content, setContent] = useState('')
  const [comparing, setComparing] = useState<ConfigVersion | null>(null)
  // 是否已用服务端产物初始化编辑器(渲染期间条件 setState,避免 effect 级联)。
  const [initialized, setInitialized] = useState(false)

  const currentQuery = useQuery({ queryKey: ['caddyfile'], queryFn: getCurrentCaddyfile })
  const versionsQuery = useQuery({ queryKey: ['versions'], queryFn: listVersions })

  if (!initialized && currentQuery.data !== undefined) {
    setContent(currentQuery.data)
    setInitialized(true)
  }

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['versions'] })
    void queryClient.invalidateQueries({ queryKey: ['caddyfile'] })
  }

  const saveMutation = useMutation({
    mutationFn: () => saveCaddyfile(content),
    onSuccess: () => {
      void message.success(t('config.saved'))
      setMode('preview')
      invalidate()
    },
    onError: (error) => {
      notification.error({
        message: t('config.saveFailed'),
        description: apiErrorMessage(error, t('config.saveFailed')),
        placement: 'bottomRight',
      })
    },
  })

  const rollbackMutation = useMutation({
    mutationFn: (id: number) => rollbackTo(id),
    onSuccess: () => {
      void message.success(t('config.rollbackSuccess'))
      setComparing(null)
      invalidate()
    },
    onError: (error) => message.error(apiErrorMessage(error, t('config.rollbackFailed'))),
  })

  const versions = versionsQuery.data ?? []

  return (
    <Row gutter={16}>
      <Col span={15}>
        <Card
          title={t('config.editorTitle')}
          extra={
            <Space>
              <Segmented
                value={mode}
                onChange={(v) => setMode(v as EditorMode)}
                options={[
                  { value: 'preview', label: t('config.mode.preview') },
                  { value: 'edit', label: t('config.mode.edit') },
                ]}
              />
              {mode === 'edit' && (
                <>
                  <Button onClick={() => setContent(currentQuery.data ?? '')}>
                    {t('common.reset')}
                  </Button>
                  <Popconfirm
                    title={t('config.saveConfirm')}
                    onConfirm={() => saveMutation.mutate()}
                  >
                    <Button type="primary" loading={saveMutation.isPending}>
                      {t('config.action.saveApply')}
                    </Button>
                  </Popconfirm>
                </>
              )}
            </Space>
          }
        >
          {comparing ? (
            <DiffEditor
              height="520px"
              original={currentQuery.data ?? ''}
              modified={comparing.rendered_caddyfile}
              language="caddyfile"
              theme="vs"
              options={{ readOnly: true, renderSideBySide: true, automaticLayout: true }}
            />
          ) : (
            <CaddyfileEditor value={content} onChange={setContent} readOnly={mode === 'preview'} />
          )}
        </Card>
      </Col>
      <Col span={9}>
        <Card
          title={t('config.versionsTitle')}
          styles={{ body: { maxHeight: 620, overflowY: 'auto' } }}
        >
          {versions.length === 0 && (
            <Typography.Text type="secondary">{t('config.noVersions')}</Typography.Text>
          )}
          <Space direction="vertical" style={{ width: '100%' }}>
            {versions.map((version) => (
              <Card
                key={version.id}
                size="small"
                title={
                  <Space>
                    <span>#{version.id}</span>
                    {version.status === 'applied' ? (
                      <Tag color="green">{t('config.status.applied')}</Tag>
                    ) : (
                      <Tag color="red">{t('config.status.failed')}</Tag>
                    )}
                  </Space>
                }
                extra={
                  <Space>
                    <Button size="small" onClick={() => setComparing(version)}>
                      {t('config.action.diff')}
                    </Button>
                    <Popconfirm
                      title={t('config.rollbackConfirm')}
                      disabled={version.status !== 'applied'}
                      onConfirm={() => rollbackMutation.mutate(version.id)}
                    >
                      <Button
                        size="small"
                        type="primary"
                        ghost
                        disabled={version.status !== 'applied'}
                        loading={
                          rollbackMutation.isPending && rollbackMutation.variables === version.id
                        }
                      >
                        {t('config.action.rollback')}
                      </Button>
                    </Popconfirm>
                  </Space>
                }
              >
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {new Date(version.created_at).toLocaleString()}
                </Typography.Text>
                {version.status === 'failed' && version.error_msg && (
                  <Typography.Paragraph
                    type="danger"
                    style={{ fontSize: 12, marginTop: 4, marginBottom: 0 }}
                  >
                    {version.error_msg}
                  </Typography.Paragraph>
                )}
              </Card>
            ))}
          </Space>
        </Card>
      </Col>
    </Row>
  )
}
