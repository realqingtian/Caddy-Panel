import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Card, Form, Input, Switch } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import { getSettings, updateSettings, type Settings } from '@/api/config'

interface SettingsFormValues {
  admin_url: string
  caddy_admin_bind: string
  caddyfile_path: string
  log_access_path: string
  log_error_path: string
  login_max_fails: string
  login_lock_window: string
  metrics_enabled: boolean
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

  return (
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
  )
}
