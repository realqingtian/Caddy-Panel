import { App, Button, Drawer, Form, Input, Select, Switch } from 'antd'
import type { FormInstance } from 'antd'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
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
          lb_policy: 'round_robin',
          https_redirect: true,
          enabled: true,
          redirect_permanent: false,
          websocket: false,
          headers_enabled: false,
          headers: [],
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
interface SiteFormValues extends Omit<SiteInput, 'custom_headers'> {
  headers?: { key: string; value: string }[]
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
  return { ...site, headers }
}

function valuesToInput(values: SiteFormValues): SiteInput {
  const { headers, ...rest } = values
  return {
    ...rest,
    custom_headers:
      values.headers_enabled && headers && headers.length > 0 ? JSON.stringify(headers) : '',
  }
}
