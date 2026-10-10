import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  App,
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
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
  'admin_url',
  'caddy_admin_bind',
  'caddyfile_path',
  'log_access_path',
  'log_error_path',
  'login_max_fails',
  'login_lock_window',
  'metrics_enabled',
  'global_extra',
]

/** 系统设置页:路径/Admin 地址/登录锁定/metrics/全局片段(FR-601)。 */
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
        <Form<SettingsFormValues>
          form={form}
          layout="vertical"
          style={{ maxWidth: 640 }}
          onFinish={(values) => saveMutation.mutate(values)}
        >
          <Form.Item
            name="admin_url"
            label={t('settings.field.adminUrl')}
            extra={t('settings.hint.adminUrl')}
          >
            <Input placeholder="http://localhost:2019" />
          </Form.Item>
          <Form.Item
            name="caddy_admin_bind"
            label={t('settings.field.adminBind')}
            extra={t('settings.hint.adminBind')}
          >
            <Input placeholder="localhost:2019" />
          </Form.Item>
          <Form.Item
            name="caddyfile_path"
            label={t('settings.field.caddyfilePath')}
            extra={t('settings.hint.nextOp')}
          >
            <Input placeholder="/etc/caddy/Caddyfile" />
          </Form.Item>
          <Form.Item
            name="log_access_path"
            label={t('settings.field.logAccess')}
            extra={t('settings.hint.nextOp')}
          >
            <Input placeholder="/data/logs/access.log" />
          </Form.Item>
          <Form.Item
            name="log_error_path"
            label={t('settings.field.logError')}
            extra={t('settings.hint.nextOp')}
          >
            <Input placeholder="/data/logs/error.log" />
          </Form.Item>
          <Form.Item
            name="login_max_fails"
            label={t('settings.field.loginMaxFails')}
            extra={t('settings.hint.immediate')}
          >
            <Input />
          </Form.Item>
          <Form.Item
            name="login_lock_window"
            label={t('settings.field.loginLockWindow')}
            extra={t('settings.hint.immediate')}
          >
            <Input placeholder="1m" />
          </Form.Item>
          <Form.Item
            name="metrics_enabled"
            label={t('settings.field.metrics')}
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="global_extra"
            label={t('settings.field.globalExtra')}
            extra={t('settings.hint.globalExtra')}
          >
            <Input.TextArea rows={4} placeholder={'servers {\n\tprotocols h1 h2\n}'} />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={saveMutation.isPending}>
            {t('common.save')}
          </Button>
        </Form>
      </Card>
      <Card title={t('settings.backup.title')}>
        <Space direction="vertical" style={{ width: '100%' }}>
          <Alert type="warning" showIcon message={t('settings.backup.warning')} />
          <Space>
            <Button onClick={() => void exportBackup()}>{t('settings.backup.export')}</Button>
            <Upload accept=".json" showUploadList={false} beforeUpload={restoreFile}>
              <Button danger>{t('settings.backup.restore')}</Button>
            </Upload>
          </Space>
          <Typography.Text type="secondary">{t('settings.backup.hint')}</Typography.Text>
        </Space>
      </Card>
      <Card
        title={t('settings.tokens.title')}
        extra={
          <Space>
            <Input
              value={newTokenName}
              onChange={(e) => setNewTokenName(e.target.value)}
              placeholder={t('settings.tokens.namePlaceholder')}
              style={{ width: 160 }}
            />
            <Select
              value={newTokenScope}
              onChange={setNewTokenScope}
              style={{ width: 100 }}
              options={[
                { value: 'read', label: 'read' },
                { value: 'write', label: 'write' },
              ]}
            />
            <Input
              value={newTokenExpiry}
              onChange={(e) => setNewTokenExpiry(e.target.value)}
              placeholder="720h"
              style={{ width: 90 }}
            />
            <Button disabled={!newTokenName.trim()} onClick={() => void runCreateToken()}>
              {t('settings.tokens.create')}
            </Button>
          </Space>
        }
      >
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
                  onConfirm={() => void runDeleteToken(record.id)}
                >
                  <Button type="link" size="small" danger>
                    {t('common.delete')}
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
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
