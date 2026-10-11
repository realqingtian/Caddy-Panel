import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { DiffEditor } from '@monaco-editor/react'
import {
  App,
  Alert,
  Button,
  Card,
  Col,
  Popconfirm,
  Row,
  Segmented,
  Space,
  Tabs,
  Tag,
  theme,
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useAuthStore } from '@/stores/auth'

import { apiErrorMessage } from '@/api/auth'
import { getCurrentCaddyfile, listVersions, saveCaddyfile, type ConfigVersion } from '@/api/config'
import { CaddyfileEditor } from '@/components/CaddyfileEditor'
import { useIsDarkMode } from '@/hooks/useThemeAlgorithm'
import { SnippetPane } from './SnippetPane'
import { VersionList } from './VersionList'

type EditorMode = 'preview' | 'edit'

/** 配置中心(logux-design §3):主配置(预览/直编+版本历史)+片段管理两个 Tab。 */
export default function ConfigEditorPage() {
  const { t } = useTranslation()
  const { message, notification } = App.useApp()
  const { token } = theme.useToken()
  const queryClient = useQueryClient()
  const role = useAuthStore((s) => s.role)

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

  const serverContent = currentQuery.data ?? ''
  // 直编未保存标记:与最近一次服务端产物比对(刷新/下发后自动消除)。
  const dirty = mode === 'edit' && content !== serverContent

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

  const resetEdit = () => {
    setContent(serverContent)
  }

  const editorCardExtra = (
    <Space>
      <Segmented
        value={mode}
        onChange={(v) => setMode(v as EditorMode)}
        options={[
          { value: 'preview', label: t('config.mode.preview') },
          { value: 'edit', label: t('config.mode.edit') },
        ]}
      />
    </Space>
  )

  return (
    <Tabs
      defaultActiveKey="main"
      items={[
        {
          key: 'main',
          label: t('config.tab.main'),
          children: (
            <Row gutter={16}>
              <Col span={15}>
                <Card title={t('config.editorTitle')} extra={editorCardExtra}>
                  {dirty && (
                    <div
                      style={{
                        position: 'sticky',
                        top: -16,
                        zIndex: 10,
                        display: 'flex',
                        justifyContent: 'flex-end',
                        alignItems: 'center',
                        gap: 8,
                        padding: '8px 12px',
                        margin: '-8px -12px 16px',
                        background: token.colorBgContainer,
                        borderBottom: `1px solid ${token.colorBorderSecondary}`,
                      }}
                    >
                      <Tag color="warning" style={{ marginInlineEnd: 0 }}>
                        {t('config.unsaved')}
                      </Tag>
                      <Button onClick={resetEdit}>{t('config.action.discard')}</Button>
                      <Popconfirm
                        title={t('config.saveConfirm')}
                        onConfirm={() => saveMutation.mutate()}
                      >
                        <Button type="primary" loading={saveMutation.isPending}>
                          {t('config.action.saveApply')}
                        </Button>
                      </Popconfirm>
                    </div>
                  )}
                  {comparing ? (
                    <>
                      <Alert
                        style={{ marginBottom: 8 }}
                        type="info"
                        showIcon
                        message={t('config.comparing', { id: comparing.id })}
                        closable
                        onClose={() => setComparing(null)}
                      />
                      <DiffViewer version={comparing} current={serverContent} />
                    </>
                  ) : (
                    // key 随对比会话变化强制重建编辑器:切回预览不残留 diff 红绿装饰(logux-design §3)。
                    <CaddyfileEditor
                      key="plain"
                      value={content}
                      onChange={setContent}
                      readOnly={mode === 'preview'}
                    />
                  )}
                </Card>
              </Col>
              <Col span={9}>
                <VersionList
                  versions={versionsQuery.data ?? []}
                  loading={versionsQuery.isLoading}
                  role={role}
                  onDiff={(version) => setComparing(version)}
                  onRolledBack={() => setComparing(null)}
                />
              </Col>
            </Row>
          ),
        },
        {
          key: 'snippets',
          label: t('config.tab.snippets'),
          children: <SnippetPane />,
        },
      ]}
    />
  )
}

/** 对比视图:左侧当前产物,右侧历史版本;主题跟随面板外观。 */
function DiffViewer({ version, current }: { version: ConfigVersion; current: string }) {
  const isDark = useIsDarkMode()
  return (
    <DiffEditor
      height="520px"
      original={current}
      modified={version.rendered_caddyfile}
      language="caddyfile"
      theme={isDark ? 'vs-dark' : 'vs'}
      options={{ readOnly: true, renderSideBySide: true, automaticLayout: true }}
    />
  )
}
