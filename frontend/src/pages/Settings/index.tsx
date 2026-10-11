import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  App,
  AutoComplete,
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  theme,
  Typography,
  Upload,
  type FormInstance,
} from 'antd'
import { useEffect, useState } from 'react'
import type { UploadFile } from 'antd'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import { apiClient } from '@/api/client'

import { tablePagination } from '@/lib/tablePagination'

import { useLocaleTag } from '@/hooks/useLocale'

import { getSettings, updateSettings, type Settings } from '@/api/config'
import { exportBackup, restoreBackup } from '@/api/backup'
import { listTokens, createToken, deleteToken, type ApiToken } from '@/api/tokens'

interface SettingsFormValues {
  gravatar_mirror_url: string
  admin_url: string
  panel_public_url: string
  caddy_admin_bind: string
  caddyfile_path: string
  log_access_path: string
  log_error_path: string
  login_max_fails: string
  login_lock_window: string
  metrics_enabled: boolean
  acme_email: string
  notify_webhook_url: string
  notify_webhook_type: string
  smtp_host: string
  smtp_port: string
  smtp_username: string
  smtp_password: string
  smtp_from: string
  smtp_to: string
  global_extra: string
}

const formKeys: (keyof SettingsFormValues)[] = [
  'gravatar_mirror_url',
  'admin_url',
  'panel_public_url',
  'caddy_admin_bind',
  'caddyfile_path',
  'log_access_path',
  'log_error_path',
  'login_max_fails',
  'login_lock_window',
  'metrics_enabled',
  'acme_email',
  'notify_webhook_url',
  'notify_webhook_type',
  'smtp_host',
  'smtp_port',
  'smtp_username',
  'smtp_password',
  'smtp_from',
  'smtp_to',
  'global_extra',
]

/** 系统设置页:Tab 分组(常规/证书与通知/API Token/备份),常规与通知为双列表单(FR-601)。 */
export default function SettingsPage() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<SettingsFormValues>()

  const settingsQuery = useQuery({ queryKey: ['settings'], queryFn: getSettings })

  useEffect(() => {
    if (!settingsQuery.data) return
    const s = settingsQuery.data
    form.setFieldsValue({
      gravatar_mirror_url: s.gravatar_mirror_url ?? '',
      admin_url: s.admin_url ?? '',
      panel_public_url: s.panel_public_url ?? '',
      caddy_admin_bind: s.caddy_admin_bind ?? '',
      caddyfile_path: s.caddyfile_path ?? '',
      log_access_path: s.log_access_path ?? '',
      log_error_path: s.log_error_path ?? '',
      login_max_fails: s.login_max_fails ?? '',
      login_lock_window: s.login_lock_window ?? '',
      metrics_enabled: s.metrics_enabled === 'true',
      acme_email: s.acme_email ?? '',
      notify_webhook_url: s.notify_webhook_url ?? '',
      notify_webhook_type: s.notify_webhook_type || 'auto',
      smtp_host: s.smtp_host ?? '',
      smtp_port: s.smtp_port ?? '',
      smtp_username: s.smtp_username ?? '',
      smtp_password: s.smtp_password ?? '',
      smtp_from: s.smtp_from ?? '',
      smtp_to: s.smtp_to ?? '',
      global_extra: s.global_extra ?? '',
    })
  }, [settingsQuery.data, form])

  const saveMutation = useMutation({
    mutationFn: (values: SettingsFormValues) => {
      const payload: Settings = {}
      for (const key of formKeys) {
        const value = values[key]
        payload[key] = typeof value === 'boolean' ? String(value) : value
      }
      return updateSettings(payload)
    },
    onSuccess: () => {
      void message.success(t('settings.saved'))
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (error) => message.error(apiErrorMessage(error, t('settings.saveFailed'))),
  })

  const tokensQuery = useQuery({
    queryKey: ['tokens'],
    queryFn: listTokens,
    enabled: settingsQuery.isSuccess,
  })
  const tokens = tokensQuery.data ?? []

  const [newTokenName, setNewTokenName] = useState('')
  const [newTokenScope, setNewTokenScope] = useState('read')
  const [newTokenExpiry, setNewTokenExpiry] = useState('')
  const [plainToken, setPlainToken] = useState<string | null>(null)

  const runCreateToken = async () => {
    try {
      const result = await createToken(newTokenName, newTokenScope, newTokenExpiry)
      setPlainToken(result.plain_token)
      setNewTokenName('')
      void tokensQuery.refetch()
    } catch (error) {
      message.error(apiErrorMessage(error, t('settings.tokens.createFailed')))
    }
  }

  const runDeleteToken = async (id: number) => {
    await deleteToken(id)
    void tokensQuery.refetch()
  }

  const restoreFile = async (file: UploadFile) => {
    try {
      const result = await restoreBackup(file.originFileObj ?? (file as unknown as File))
      void message.success(
        t('settings.backup.restored', { sites: result.sites, versions: result.versions }),
      )
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    } catch (error) {
      message.error(apiErrorMessage(error, t('settings.backup.restoreFailed')))
    }
    return false
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card title={t('settings.title')}>
        {/* Form 包裹 Tabs:两个表单 Tab 共用一个 form 实例;forceRender 保证未激活 tab 的字段参与提交 */}
        <Form<SettingsFormValues>
          form={form}
          layout="vertical"
          onFinish={(values) => saveMutation.mutate(values)}
        >
          <Tabs
            tabPosition="left"
            items={[
              {
                key: 'caddy',
                label: t('settings.cat.caddy'), // 连接与部署
                children: <CaddyPane savePending={saveMutation.isPending} form={form} />,
              },
              {
                key: 'security',
                label: t('settings.cat.security'),
                children: <SecurityPane savePending={saveMutation.isPending} />,
              },
              {
                key: 'notify',
                label: t('settings.cat.notify'),
                forceRender: true,
                children: <NotifyPane savePending={saveMutation.isPending} />,
              },
              {
                key: 'tokens',
                label: t('settings.tokens.title'),
                children: (
                  <TokensPane
                    tokens={tokens}
                    newTokenName={newTokenName}
                    onNewTokenName={setNewTokenName}
                    newTokenScope={newTokenScope}
                    onNewTokenScope={setNewTokenScope}
                    newTokenExpiry={newTokenExpiry}
                    onNewTokenExpiry={setNewTokenExpiry}
                    onCreate={() => void runCreateToken()}
                    onDelete={(id) => void runDeleteToken(id)}
                  />
                ),
              },
              {
                key: 'backup',
                label: t('settings.backup.title'),
                children: <BackupPane onRestore={restoreFile} />,
              },
              {
                key: 'avatar',
                label: t('settings.cat.avatar'),
                children: <AvatarPane savePending={saveMutation.isPending} />,
              },
              {
                key: 'advanced',
                label: t('settings.cat.advanced'),
                children: <AdvancedPane savePending={saveMutation.isPending} />,
              },
            ]}
          />
        </Form>
      </Card>
      <Modal
        open={plainToken !== null}
        title={t('settings.tokens.plainTitle')}
        footer={
          <Button type="primary" onClick={() => setPlainToken(null)}>
            {t('common.close')}
          </Button>
        }
        onCancel={() => setPlainToken(null)}
      >
        <Alert
          type="warning"
          showIcon
          message={t('settings.tokens.plainWarning')}
          style={{ marginBottom: 12 }}
        />
        <Typography.Paragraph copyable style={{ fontFamily: 'monospace' }}>
          {plainToken}
        </Typography.Paragraph>
      </Modal>
    </Space>
  )
}

// Gravatar 镜像预设(I4-D3):空 = 官方源;须兼容 Gravatar URL 协议
const GRAVATAR_PRESETS = [
  'https://gravatar.com',
  'https://gravatar.qingtian.dev',
  'https://cravatar.cn',
  'https://weavatar.com',
  'https://loli.net',
]

/** 连接与部署:面板对外地址可编辑;Caddy 连接与路径为部署级配置,只读展示(误改即失联,I6 评审)。 */
function CaddyPane({ savePending, form }: { savePending: boolean; form: FormInstance }) {
  const { t } = useTranslation()
  const ro = (label: string, value: string) => (
    <Col span={12}>
      <Form.Item label={label} style={{ marginBottom: 8 }}>
        <Typography.Text copyable style={{ fontFamily: 'monospace' }}>
          {value}
        </Typography.Text>
      </Form.Item>
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        {t('settings.hint.managedByEnv')}
      </Typography.Text>
    </Col>
  )
  return (
    <>
      <SaveButton pending={savePending} />
      <Alert
        type="info"
        showIcon
        message={t('settings.cat.caddy')}
        description={t('settings.hint.deployManaged')}
        style={{ marginBottom: 24 }}
      />
      <Row gutter={24}>
        <Col span={12}>
          <Form.Item
            name="panel_public_url"
            label={t('settings.field.panelPublic')}
            extra={t('settings.hint.panelPublic')}
          >
            <Input placeholder="https://panel.example.com" />
          </Form.Item>
        </Col>
        <Col span={12} />
      </Row>
      <Divider titlePlacement="start">{t('settings.cat.deployment')}</Divider>
      <Row gutter={24}>
        {ro(t('settings.field.adminUrl'), String(form.getFieldValue('admin_url') || ''))}
        {ro(t('settings.field.adminBind'), String(form.getFieldValue('caddy_admin_bind') || ''))}
        {ro(t('settings.field.caddyfilePath'), String(form.getFieldValue('caddyfile_path') || ''))}
        {ro(t('settings.field.logAccess'), String(form.getFieldValue('log_access_path') || ''))}
        {ro(t('settings.field.logError'), String(form.getFieldValue('log_error_path') || ''))}
      </Row>
    </>
  )
}

/** 安全:登录失败锁定(修改后立即生效)。 */
function SecurityPane({ savePending }: { savePending: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <SaveButton pending={savePending} />
      <Row gutter={24}>
        <Col span={12}>
          <Form.Item
            name="login_max_fails"
            label={t('settings.field.loginMaxFails')}
            extra={t('settings.hint.immediate')}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item
            name="login_lock_window"
            label={t('settings.field.loginLockWindow')}
            extra={t('settings.hint.immediate')}
          >
            <Input placeholder="1m" />
          </Form.Item>
        </Col>
      </Row>
    </>
  )
}

/** 头像:Gravatar 镜像(自定义上传头像在个人设置页)。 */
function AvatarPane({ savePending }: { savePending: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <SaveButton pending={savePending} />
      <Row gutter={24}>
        <Col span={12}>
          <Form.Item
            name="gravatar_mirror_url"
            label={t('settings.field.gravatarMirror')}
            extra={t('settings.hint.gravatarMirror')}
          >
            <AutoComplete
              options={GRAVATAR_PRESETS.map((url) => ({ value: url }))}
              placeholder="https://gravatar.com"
              allowClear
            />
          </Form.Item>
        </Col>
        <Col span={12} />
      </Row>
    </>
  )
}

/** 高级:metrics 开关与全局自定义片段。 */
function AdvancedPane({ savePending }: { savePending: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <SaveButton pending={savePending} />
      <Form.Item name="metrics_enabled" label={t('settings.field.metrics')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Form.Item
        name="global_extra"
        label={t('settings.field.globalExtra')}
        extra={t('settings.hint.globalExtra')}
      >
        <Input.TextArea rows={4} placeholder={'servers {\n\tprotocols h1 h2\n}'} />
      </Form.Item>
    </>
  )
}

function NotifyPane({ savePending }: { savePending: boolean }) {
  const { t } = useTranslation()
  const { message } = App.useApp()

  const testWebhook = () => {
    void apiClient
      .post('/settings/webhook/test')
      .then(() => message.success(t('settings.webhookTestOk')))
      .catch((error) => message.error(apiErrorMessage(error, t('settings.webhookTestFailed'))))
  }
  const testSMTP = () => {
    void apiClient
      .post('/settings/smtp/test')
      .then(() => message.success(t('settings.smtpTestOk')))
      .catch((error) => message.error(apiErrorMessage(error, t('settings.smtpTestFailed'))))
  }

  const typeOptions = [
    { value: 'auto', label: t('settings.webhookType.auto') },
    { value: 'feishu', label: t('settings.webhookType.feishu') },
    { value: 'dingtalk', label: t('settings.webhookType.dingtalk') },
    { value: 'wecom', label: t('settings.webhookType.wecom') },
    { value: 'slack', label: t('settings.webhookType.slack') },
    { value: 'discord', label: t('settings.webhookType.discord') },
    { value: 'json', label: t('settings.webhookType.json') },
  ]

  return (
    <>
      <SaveButton pending={savePending} />
      {/* 事件源配置:ACME 账号 + 到期 Webhook 地址 */}
      <Row gutter={24}>
        <Col span={12}>
          <Form.Item
            name="acme_email"
            label={t('settings.field.acmeEmail')}
            extra={t('settings.hint.acmeEmail')}
          >
            <Input placeholder="admin@example.com" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item
            name="notify_webhook_url"
            label={t('settings.field.notifyWebhook')}
            extra={t('settings.hint.notifyWebhook')}
          >
            <Input placeholder="https://example.com/hook" />
          </Form.Item>
        </Col>
      </Row>

      {/* Webhook 渠道:类型与测试按钮同区 */}
      <Divider titlePlacement="start">{t('settings.webhookChannel')}</Divider>
      <Row gutter={24} align="bottom">
        <Col span={12}>
          <Form.Item name="notify_webhook_type" label={t('settings.field.notifyWebhookType')}>
            <Select options={typeOptions} />
          </Form.Item>
        </Col>
        <Col span={12} style={{ marginBottom: 24, display: 'flex' }}>
          <Button onClick={testWebhook}>{t('settings.webhookTest')}</Button>
        </Col>
      </Row>

      {/* SMTP 渠道:表单与测试按钮同区 */}
      <Divider titlePlacement="start">{t('settings.smtp.title')}</Divider>
      <Row gutter={24}>
        <Col span={12}>
          <Form.Item name="smtp_host" label={t('settings.field.smtpHost')}>
            <Input placeholder="smtp.example.com" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item name="smtp_port" label={t('settings.field.smtpPort')}>
            <Input placeholder="465" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item name="smtp_username" label={t('settings.field.smtpUsername')}>
            <Input autoComplete="off" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item
            name="smtp_password"
            label={t('settings.field.smtpPassword')}
            extra={t('settings.hint.smtpPassword')}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item name="smtp_from" label={t('settings.field.smtpFrom')}>
            <Input placeholder="panel@example.com" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item
            name="smtp_to"
            label={t('settings.field.smtpTo')}
            extra={t('settings.hint.smtpTo')}
          >
            <Input placeholder="ops@example.com" />
          </Form.Item>
        </Col>
      </Row>
      <Button style={{ marginBottom: 24 }} onClick={testSMTP}>
        {t('settings.smtpTest')}
      </Button>
    </>
  )
}

/** 保存栏:吸顶右对齐——长表单滚动时保存始终可见,不再沉在页面底部(I6 体验反馈)。 */
function SaveButton({ pending }: { pending: boolean }) {
  const { t } = useTranslation()
  const { token } = theme.useToken()
  return (
    <div
      style={{
        position: 'sticky',
        top: -16,
        zIndex: 10,
        display: 'flex',
        justifyContent: 'flex-end',
        alignItems: 'center',
        padding: '8px 12px',
        margin: '-8px -12px 16px',
        background: token.colorBgContainer,
        borderBottom: `1px solid ${token.colorBorderSecondary}`,
      }}
    >
      <Button type="primary" htmlType="submit" loading={pending}>
        {t('common.save')}
      </Button>
    </div>
  )
}

interface TokensPaneProps {
  tokens: ApiToken[]
  newTokenName: string
  onNewTokenName: (v: string) => void
  newTokenScope: string
  onNewTokenScope: (v: string) => void
  newTokenExpiry: string
  onNewTokenExpiry: (v: string) => void
  onCreate: () => void
  onDelete: (id: number) => void
}

/** API Token:签发 + 列表管理(仅 JWT+admin 可见,页面入口已由 RBAC 控制)。 */
function TokensPane({
  tokens,
  newTokenName,
  onNewTokenName,
  newTokenScope,
  onNewTokenScope,
  newTokenExpiry,
  onNewTokenExpiry,
  onCreate,
  onDelete,
}: TokensPaneProps) {
  const { t } = useTranslation()
  const localeTag = useLocaleTag()
  return (
    <>
      <Space style={{ marginBottom: 12 }} wrap>
        <Input
          value={newTokenName}
          onChange={(e) => onNewTokenName(e.target.value)}
          placeholder={t('settings.tokens.namePlaceholder')}
          style={{ width: 160 }}
        />
        <Select
          value={newTokenScope}
          onChange={onNewTokenScope}
          style={{ width: 100 }}
          options={[
            { value: 'read', label: 'read' },
            { value: 'write', label: 'write' },
          ]}
        />
        <Input
          value={newTokenExpiry}
          onChange={(e) => onNewTokenExpiry(e.target.value)}
          placeholder="720h"
          style={{ width: 90 }}
        />
        <Button disabled={!newTokenName.trim()} onClick={onCreate}>
          {t('settings.tokens.create')}
        </Button>
      </Space>
      <Alert
        type="info"
        showIcon
        message={t('settings.tokens.hint')}
        style={{ marginBottom: 12 }}
      />
      <Table<ApiToken>
        pagination={tablePagination(t)}
        rowKey="id"
        size="small"
        dataSource={tokens}
        columns={[
          { title: t('settings.tokens.column.name'), dataIndex: 'name' },
          { title: t('settings.tokens.column.prefix'), dataIndex: 'prefix' },
          { title: t('settings.tokens.column.scope'), dataIndex: 'scopes' },
          {
            title: t('settings.tokens.column.lastUsed'),
            dataIndex: 'last_used_at',
            render: (v: string | null) => (v ? new Date(v).toLocaleString(localeTag) : '—'),
          },
          {
            title: t('common.actions'),
            key: 'actions',
            render: (_: unknown, record: ApiToken) => (
              <Popconfirm
                title={t('settings.tokens.deleteConfirm')}
                onConfirm={() => onDelete(record.id)}
              >
                <Button type="link" size="small" danger>
                  {t('common.delete')}
                </Button>
              </Popconfirm>
            ),
          },
        ]}
      />
    </>
  )
}

/** 备份与恢复:导出 JSON / 导入恢复(不自动下发)。 */
function BackupPane({ onRestore }: { onRestore: (file: UploadFile) => Promise<boolean> }) {
  const { t } = useTranslation()
  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <Alert type="warning" showIcon message={t('settings.backup.warning')} />
      <Space>
        <Button onClick={() => void exportBackup()}>{t('settings.backup.export')}</Button>
        <Upload accept=".json" showUploadList={false} beforeUpload={onRestore}>
          <Button danger>{t('settings.backup.restore')}</Button>
        </Upload>
      </Space>
      <Typography.Text type="secondary">{t('settings.backup.hint')}</Typography.Text>
    </Space>
  )
}
