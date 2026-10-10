import { App, Button, Drawer, Form, Input, Select, Switch } from 'antd'
import type { FormInstance } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'

import { apiErrorMessage } from '@/api/auth'
import { listNodes } from '@/api/nodes'
import { createSite, updateSite, type Site, type SiteInput } from '@/api/sites'

interface SiteFormProps {
  open: boolean
  /** 编辑目标;null 表示新建。 */
  site: Site | null
  onClose: () => void
  onSaved: () => void
}

/** Drawer 内的站点表单:按类型切换动态字段(FR-301/302/303/306/307)。 */
export function SiteForm({ open, site, onClose, onSaved }: SiteFormProps) {
  const { t } = useTranslation()
  const { message, notification } = App.useApp()
  const [form] = Form.useForm<SiteFormValues>()

  useEffect(() => {
    if (!open) return
    if (site) {
      form.setFieldsValue(siteToValues(site))
    } else {
      form.resetFields()
    }
  }, [open, site, form])

  const onSubmit = async (values: SiteFormValues) => {
    const input = valuesToInput(values)
    try {
      if (site) {
        await updateSite(site.id, input)
      } else {
        await createSite(input)
      }
      void message.success(t('sites.saved'))
      onSaved()
      onClose()
    } catch (error) {
      // 校验/下发失败:message 提示 + notification 展示 Caddy 错误原文(FR-304)。
      notification.error({
        message: t('sites.saveFailed'),
        description: apiErrorMessage(error, t('sites.saveFailed')),
        placement: 'bottomRight',
      })
    }
  }

  const type = Form.useWatch('type', form) ?? 'proxy'

  // 节点选择(local + 已接入节点);编辑时禁用:归属节点不可迁移。
  const nodesQuery = useQuery({ queryKey: ['nodes'], queryFn: listNodes, enabled: open && !site })
  const nodeOptions = [
    { value: 'local', label: t('sites.node.local') },
    ...(nodesQuery.data ?? []).map((n) => ({ value: n.node_id, label: `${n.name} (${n.status})` })),
  ]

  return (
    <Drawer
      open={open}
      title={site ? t('sites.editTitle') : t('sites.newTitle')}
      width={520}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Button type="primary" onClick={() => form.submit()}>
          {t('common.save')}
        </Button>
      }
    >
      <Form<SiteFormValues>
        form={form}
        layout="vertical"
        onFinish={onSubmit}
        initialValues={{
          type: 'proxy',
          node_id: 'local',
          lb_policy: 'round_robin',
          https_redirect: true,
          enabled: true,
          redirect_permanent: false,
          websocket: false,
          headers_enabled: false,
          headers: [],
          basic_auth: [],
          ip_allowlist: '',
          ip_blocklist: '',
          rate_limit_events: 100,
          rate_limit_window: '1m',
          dns_provider: '',
          dns_token_env: '',
        }}
      >
        <Form.Item
          name="name"
          label={t('sites.field.name')}
          rules={[{ required: true, message: t('sites.required.name') }]}
        >
          <Input maxLength={128} />
        </Form.Item>
        <Form.Item name="type" label={t('sites.field.type')} rules={[{ required: true }]}>
          <Select
            options={[
              { value: 'proxy', label: t('sites.type.proxy') },
              { value: 'static', label: t('sites.type.static') },
              { value: 'redirect', label: t('sites.type.redirect') },
            ]}
          />
        </Form.Item>
        <Form.Item
          name="node_id"
          label={t('sites.field.node')}
          extra={site ? t('sites.hint.nodeLocked') : t('sites.hint.node')}
        >
          <Select disabled={!!site} options={nodeOptions} />
        </Form.Item>
        <Form.Item
          name="domains"
          label={t('sites.field.domains')}
          extra={t('sites.hint.domains')}
          rules={[{ required: true, message: t('sites.required.domains') }]}
        >
          <Input.TextArea rows={2} />
        </Form.Item>

        {type === 'proxy' && (
          <>
            <Form.Item
              name="upstreams"
              label={t('sites.field.upstreams')}
              extra={t('sites.hint.upstreams')}
              rules={[{ required: true, message: t('sites.required.upstreams') }]}
            >
              <Input.TextArea rows={2} />
            </Form.Item>
            <Form.Item name="lb_policy" label={t('sites.field.lbPolicy')}>
              <Select
                options={[
                  { value: 'round_robin', label: t('sites.lbPolicy.roundRobin') },
                  { value: 'least_conn', label: t('sites.lbPolicy.leastConn') },
                  { value: 'ip_hash', label: t('sites.lbPolicy.ipHash') },
                  { value: 'first', label: t('sites.lbPolicy.first') },
                ]}
              />
            </Form.Item>
            <Form.Item
              name="health_check_enabled"
              label={t('sites.field.healthCheck')}
              valuePropName="checked"
            >
              <Switch />
            </Form.Item>
            <HealthCheckFields form={form} />
            <Form.Item
              name="rate_limit_enabled"
              label={t('sites.field.rateLimit')}
              tooltip={t('sites.hint.rateLimit')}
              valuePropName="checked"
            >
              <Switch />
            </Form.Item>
            <RateLimitFields form={form} />
            <Form.Item name="websocket" label={t('sites.field.websocket')} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item
              name="headers_enabled"
              label={t('sites.field.headersEnabled')}
              valuePropName="checked"
            >
              <Switch />
            </Form.Item>
            <HeaderList form={form} />
          </>
        )}

        {type === 'static' && (
          <Form.Item
            name="root_path"
            label={t('sites.field.rootPath')}
            rules={[{ required: true, message: t('sites.required.rootPath') }]}
          >
            <Input placeholder="/var/www/html" />
          </Form.Item>
        )}

        {type === 'redirect' && (
          <>
            <Form.Item
              name="redirect_to"
              label={t('sites.field.redirectTo')}
              rules={[{ required: true, message: t('sites.required.redirectTo') }]}
            >
              <Input placeholder="https://new.example.com" />
            </Form.Item>
            <Form.Item
              name="redirect_permanent"
              label={t('sites.field.redirectPermanent')}
              valuePropName="checked"
            >
              <Switch />
            </Form.Item>
          </>
        )}

        <AccessControlFields />
        <Form.Item
          name="dns_provider"
          label={t('sites.field.dnsProvider')}
          tooltip={t('sites.hint.dnsProvider')}
        >
          <Select
            allowClear
            placeholder={t('sites.field.dnsProviderPlaceholder')}
            options={[{ value: 'cloudflare', label: 'Cloudflare' }]}
          />
        </Form.Item>
        <Form.Item
          name="dns_token_env"
          label={t('sites.field.dnsTokenEnv')}
          extra={t('sites.hint.dnsTokenEnv')}
        >
          <Input placeholder="CF_API_TOKEN" />
        </Form.Item>
        <Form.Item
          name="https_redirect"
          label={t('sites.field.httpsRedirect')}
          valuePropName="checked"
        >
          <Switch />
        </Form.Item>
        <Form.Item name="enabled" label={t('sites.field.enabled')} valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>
    </Drawer>
  )
}

/** 表单值:custom_headers 以 Form.List 结构编辑,提交时序列化为 JSON。 */
interface SiteFormValues extends Omit<SiteInput, 'basic_auth' | 'custom_headers'> {
  headers?: { key: string; value: string }[]
  basic_auth?: { username: string; password: string }[]
}

/** 自定义响应头 kv 编辑器;仅在 headers_enabled 开启时渲染。 */
function HeaderList({ form }: { form: FormInstance<SiteFormValues> }) {
  const { t } = useTranslation()
  const enabled = Form.useWatch('headers_enabled', form)
  if (!enabled) return null

  return (
    <Form.List name="headers">
      {(fields, { add, remove }) => (
        <div style={{ marginBottom: 24 }}>
          {fields.map((field) => (
            <div key={field.key} style={{ display: 'flex', gap: 8 }}>
              <Form.Item
                name={[field.name, 'key']}
                rules={[{ required: true, message: t('sites.required.headerKey') }]}
                style={{ flex: 2, marginBottom: 8 }}
              >
                <Input placeholder={t('sites.field.headerKey')} />
              </Form.Item>
              <Form.Item name={[field.name, 'value']} style={{ flex: 3, marginBottom: 8 }}>
                <Input placeholder={t('sites.field.headerValue')} />
              </Form.Item>
              <Button type="text" danger onClick={() => remove(field.name)}>
                {t('common.delete')}
              </Button>
            </div>
          ))}
          <Button type="dashed" block onClick={() => add({ key: '', value: '' })}>
            {t('sites.action.addHeader')}
          </Button>
        </div>
      )}
    </Form.List>
  )
}

function siteToValues(site: Site): SiteFormValues {
  let headers: { key: string; value: string }[]
  try {
    headers = JSON.parse(site.custom_headers) as { key: string; value: string }[]
  } catch {
    headers = []
  }
  // 已有用户名回显;密码不回显(哈希不可逆),重输即重置密码。
  const basic_auth = (
    site.basic_auth_users ? (JSON.parse(site.basic_auth_users) as { username: string }[]) : []
  ).map((u) => ({ username: u.username, password: '' }))
  return { ...site, headers, basic_auth, node_id: site.node_id || 'local' }
}

function valuesToInput(values: SiteFormValues): SiteInput {
  const { headers, basic_auth, ...rest } = values
  return {
    ...rest,
    custom_headers:
      values.headers_enabled && headers && headers.length > 0 ? JSON.stringify(headers) : '',
    basic_auth: (basic_auth ?? []).filter((u) => u.username),
  }
}

/** 健康检查参数;仅在开关打开时渲染(FR-701)。 */
function HealthCheckFields({ form }: { form: FormInstance<SiteFormValues> }) {
  const { t } = useTranslation()
  const enabled = Form.useWatch('health_check_enabled', form)
  if (!enabled) return null

  return (
    <div style={{ display: 'flex', gap: 8 }}>
      <Form.Item name="health_check_path" style={{ flex: 2 }}>
        <Input placeholder={t('sites.field.healthCheckPath')} />
      </Form.Item>
      <Form.Item name="health_check_interval" style={{ flex: 1 }}>
        <Input placeholder="10s" />
      </Form.Item>
      <Form.Item name="health_check_expected" style={{ flex: 1 }}>
        <Input type="number" placeholder="200" />
      </Form.Item>
    </div>
  )
}

/** 访问控制区块(FR-704):basic auth 用户与 IP 名单,三类站点通用。 */
function AccessControlFields() {
  const { t } = useTranslation()
  return (
    <>
      <Form.Item label={t('sites.field.basicAuth')} tooltip={t('sites.hint.basicAuth')}>
        <Form.List name="basic_auth">
          {(fields, { add, remove }) => (
            <div>
              {fields.map((field) => (
                <div key={field.key} style={{ display: 'flex', gap: 8 }}>
                  <Form.Item
                    name={[field.name, 'username']}
                    rules={[{ required: true, message: t('sites.required.basicAuthUser') }]}
                    style={{ flex: 1, marginBottom: 8 }}
                  >
                    <Input placeholder={t('sites.field.basicAuthUser')} />
                  </Form.Item>
                  <Form.Item
                    name={[field.name, 'password']}
                    rules={[{ required: true, message: t('sites.required.basicAuthPassword') }]}
                    style={{ flex: 1, marginBottom: 8 }}
                  >
                    <Input.Password
                      placeholder={t('sites.field.basicAuthPassword')}
                      autoComplete="new-password"
                    />
                  </Form.Item>
                  <Button type="text" danger onClick={() => remove(field.name)}>
                    {t('common.delete')}
                  </Button>
                </div>
              ))}
              <Button type="dashed" block onClick={() => add({ username: '', password: '' })}>
                {t('sites.action.addBasicAuth')}
              </Button>
            </div>
          )}
        </Form.List>
      </Form.Item>
      <Form.Item
        name="ip_allowlist"
        label={t('sites.field.ipAllowlist')}
        extra={t('sites.hint.ipList')}
      >
        <Input.TextArea rows={2} placeholder="203.0.113.0/24, 10.0.0.5" />
      </Form.Item>
      <Form.Item
        name="ip_blocklist"
        label={t('sites.field.ipBlocklist')}
        extra={t('sites.hint.ipList')}
      >
        <Input.TextArea rows={2} />
      </Form.Item>
    </>
  )
}

/** 限流参数;仅在开关打开时渲染(需带 caddy-rate-limit 插件的 Caddy)。 */
function RateLimitFields({ form }: { form: FormInstance<SiteFormValues> }) {
  const { t } = useTranslation()
  const enabled = Form.useWatch('rate_limit_enabled', form)
  if (!enabled) return null

  return (
    <div style={{ display: 'flex', gap: 8 }}>
      <Form.Item name="rate_limit_events" style={{ flex: 1 }}>
        <Input type="number" placeholder={t('sites.field.rateLimitEvents')} />
      </Form.Item>
      <Form.Item name="rate_limit_window" style={{ flex: 1 }}>
        <Input placeholder="1m" />
      </Form.Item>
    </div>
  )
}
