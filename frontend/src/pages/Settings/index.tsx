import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  App,
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
  Typography,
  Upload,
} from 'antd'
import { useEffect, useState } from 'react'
import type { UploadFile } from 'antd'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import { getSettings, updateSettings, type Settings } from '@/api/config'
import { exportBackup, restoreBackup } from '@/api/backup'
import { listTokens, createToken, deleteToken, type ApiToken } from '@/api/tokens'

interface SettingsFormValues {
  gravatar_mirror_url: string
  admin_url: string
  caddy_admin_bind: string
  caddyfile_path: string
  log_access_path: string
  log_error_path: string
  login_max_fails: string
  login_lock_window: string
  metrics_enabled: boolean
  acme_email: string
  notify_webhook_url: string
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
  'caddy_admin_bind',
  'caddyfile_path',
  'log_access_path',
  'log_error_path',
  'login_max_fails',
  'login_lock_window',
  'metrics_enabled',
  'acme_email',
  'notify_webhook_url',
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
      caddy_admin_bind: s.caddy_admin_bind ?? '',
      caddyfile_path: s.caddyfile_path ?? '',
      log_access_path: s.log_access_path ?? '',
      log_error_path: s.log_error_path ?? '',
      login_max_fails: s.login_max_fails ?? '',
      login_lock_window: s.login_lock_window ?? '',
      metrics_enabled: s.metrics_enabled === 'true',
      acme_email: s.acme_email ?? '',
      notify_webhook_url: s.notify_webhook_url ?? '',
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
            items={[
              {
                key: 'general',
                label: t('settings.tab.general'),
                children: <GeneralPane savePending={saveMutation.isPending} />,
              },
              {
                key: 'notify',
                label: t('settings.tab.notify'),
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

/** 常规:Caddy 连接 / 路径 / 登录安全 / metrics / 全局片段,双列布局。 */
function GeneralPane({ savePending }: { savePending: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <Divider titlePlacement="start">{t('settings.group.connection')}</Divider>
      <Row gutter={24}>
        <Col span={12}>
          <Form.Item
            name="admin_url"
            label={t('settings.field.adminUrl')}
            extra={t('settings.hint.adminUrl')}
          >
            <Input placeholder="http://localhost:2019" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item
            name="caddy_admin_bind"
            label={t('settings.field.adminBind')}
            extra={t('settings.hint.adminBind')}
          >
            <Input placeholder="localhost:2019" />
          </Form.Item>
        </Col>
      </Row>
      <Divider titlePlacement="start">{t('settings.group.paths')}</Divider>
      <Row gutter={24}>
        <Col span={12}>
          <Form.Item
            name="caddyfile_path"
            label={t('settings.field.caddyfilePath')}
            extra={t('settings.hint.nextOp')}
          >
            <Input placeholder="/etc/caddy/Caddyfile" />
          </Form.Item>
        </Col>
        <Col span={12} />
        <Col span={12}>
          <Form.Item
            name="log_access_path"
            label={t('settings.field.logAccess')}
            extra={t('settings.hint.nextOp')}
          >
            <Input placeholder="/data/logs/access.log" />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item
            name="log_error_path"
            label={t('settings.field.logError')}
            extra={t('settings.hint.nextOp')}
          >
            <Input placeholder="/data/logs/error.log" />
          </Form.Item>
        </Col>
      </Row>
      <Divider titlePlacement="start">{t('settings.group.security')}</Divider>
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
      <Form.Item name="metrics_enabled" label={t('settings.field.metrics')} valuePropName="checked">
        <Switch />
      </Form.Item>
      <Divider titlePlacement="start">{t('settings.group.advanced')}</Divider>
      <Form.Item
        name="global_extra"
        label={t('settings.field.globalExtra')}
        extra={t('settings.hint.globalExtra')}
      >
        <Input.TextArea rows={4} placeholder={'servers {\n\tprotocols h1 h2\n}'} />
      </Form.Item>
      <SaveButton pending={savePending} />
    </>
  )
}

/** 证书与通知:ACME 邮箱 / 到期 Webhook / SMTP 渠道(M2 起后端支持但 UI 首次补齐,见 iterations I3-4)。 */
function NotifyPane({ savePending }: { savePending: boolean }) {
  const { t } = useTranslation()
  return (
    <>
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
      <SaveButton pending={savePending} />
    </>
  )
}

function SaveButton({ pending }: { pending: boolean }) {
  const { t } = useTranslation()
  return (
    <Button type="primary" htmlType="submit" loading={pending}>
      {t('common.save')}
    </Button>
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
        rowKey="id"
        size="small"
        dataSource={tokens}
        pagination={false}
        columns={[
          { title: t('settings.tokens.column.name'), dataIndex: 'name' },
          { title: t('settings.tokens.column.prefix'), dataIndex: 'prefix' },
          { title: t('settings.tokens.column.scope'), dataIndex: 'scopes' },
          {
            title: t('settings.tokens.column.lastUsed'),
            dataIndex: 'last_used_at',
            render: (v: string | null) => (v ? new Date(v).toLocaleString() : '—'),
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
