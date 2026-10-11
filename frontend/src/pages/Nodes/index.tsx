import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  App,
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  type TableProps,
} from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { tablePagination } from '@/lib/tablePagination'

import { useLocaleTag } from '@/hooks/useLocale'

import { apiErrorMessage } from '@/api/auth'
import {
  createEnrollToken,
  deleteEnrollToken,
  deleteNode,
  getUninstallCommand,
  listEnrollTokens,
  listNodes,
  revokeEnrollToken,
  type EnrollmentToken,
  type Node,
} from '@/api/nodes'

const { Paragraph, Text } = Typography

/** 时间本地化展示;空值占位。 */
const formatTimeLocaled = (v: string, locale: string) => new Date(v).toLocaleString(locale)

/** 节点管理页:节点列表、接入向导(令牌 + 安装命令)与令牌管理。 */
export default function NodesPage() {
  const { t } = useTranslation()
  const localeTag = useLocaleTag()
  const { message } = App.useApp()
  const queryClient = useQueryClient()

  const nodesQuery = useQuery({ queryKey: ['nodes'], queryFn: listNodes })
  const tokensQuery = useQuery({ queryKey: ['enroll-tokens'], queryFn: listEnrollTokens })

  const [enrollOpen, setEnrollOpen] = useState(false)
  const [enrollResult, setEnrollResult] = useState<{ token: string; install: string } | null>(null)
  const [uninstallOpen, setUninstallOpen] = useState(false)
  const [uninstallCmd, setUninstallCmd] = useState('')

  const nodeColumns: TableProps<Node>['columns'] = [
    { title: t('nodes.field.name'), dataIndex: 'name' },
    {
      title: t('nodes.field.status'),
      dataIndex: 'status',
      render: (status: Node['status']) => (
        <Tag color={status === 'online' ? 'success' : 'default'}>{t('nodes.status.' + status)}</Tag>
      ),
    },
    {
      title: t('nodes.field.health'),
      render: (_, node) => (
        <Space size={4} wrap>
          <Tag color={node.caddy_ok ? 'success' : 'error'}>{t('nodes.caddy')}</Tag>
          <Tag color={node.in_sync ? 'success' : 'warning'}>{t('nodes.inSync')}</Tag>
        </Space>
      ),
    },
    { title: t('nodes.field.sites'), dataIndex: 'site_count', width: 80 },
    { title: t('nodes.field.version'), dataIndex: 'agent_version', width: 100 },
    { title: t('nodes.field.platform'), dataIndex: 'platform', width: 130 },
    {
      title: t('nodes.field.lastSeen'),
      dataIndex: 'last_seen_at',
      render: (v: string | null) => (v ? formatTimeLocaled(v, localeTag) : '-'),
    },
    {
      title: t('common.actions'),
      render: (_, node) => (
        <Popconfirm
          title={t('nodes.deleteConfirm')}
          onConfirm={() => removeNode(node)}
          disabled={node.site_count > 0}
        >
          <Button danger size="small" disabled={node.site_count > 0}>
            {t('common.delete')}
          </Button>
        </Popconfirm>
      ),
    },
  ]

  const tokenColumns: TableProps<EnrollmentToken>['columns'] = [
    { title: t('nodes.enroll.nodeName'), dataIndex: 'node_name' },
    { title: t('nodes.field.platform'), dataIndex: 'platform' },
    {
      title: t('nodes.enroll.prefix'),
      dataIndex: 'prefix',
      render: (v: string) => <Text code>{v}…</Text>,
    },
    {
      title: t('nodes.enroll.state'),
      render: (_, token) =>
        token.used_at ? (
          <Tag color="default">{t('nodes.enroll.used')}</Tag>
        ) : token.revoked ? (
          <Tag color="error">{t('nodes.enroll.revoked')}</Tag>
        ) : new Date(token.expires_at) < new Date() ? (
          <Tag color="warning">{t('nodes.enroll.expired')}</Tag>
        ) : (
          <Tag color="processing">{t('nodes.enroll.pending')}</Tag>
        ),
    },
    {
      title: t('nodes.enroll.expiresAt'),
      dataIndex: 'expires_at',
      render: (v: string) => formatTimeLocaled(v, localeTag),
    },
    {
      title: t('common.actions'),
      render: (_, token) => (
        <Space>
          {!token.used_at && !token.revoked && (
            <Button size="small" onClick={() => runRevoke(token)}>
              {t('nodes.enroll.revoke')}
            </Button>
          )}
          <Button size="small" danger onClick={() => runDeleteToken(token)}>
            {t('common.delete')}
          </Button>
        </Space>
      ),
    },
  ]

  const removeNode = async (node: Node) => {
    try {
      await deleteNode(node.node_id)
      void message.success(t('nodes.deleted'))
      void queryClient.invalidateQueries({ queryKey: ['nodes'] })
    } catch (error) {
      message.error(apiErrorMessage(error, t('nodes.deleteFailed')))
    }
  }

  const runRevoke = async (token: EnrollmentToken) => {
    await revokeEnrollToken(token.id)
    void queryClient.invalidateQueries({ queryKey: ['enroll-tokens'] })
  }

  const runDeleteToken = async (token: EnrollmentToken) => {
    await deleteEnrollToken(token.id)
    void queryClient.invalidateQueries({ queryKey: ['enroll-tokens'] })
  }

  const showUninstall = async () => {
    setUninstallCmd(await getUninstallCommand())
    setUninstallOpen(true)
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card
        title={t('nodes.title')}
        extra={
          <Space>
            <Button onClick={() => void showUninstall()}>{t('nodes.uninstall')}</Button>
            <Button
              type="primary"
              onClick={() => {
                setEnrollResult(null)
                setEnrollOpen(true)
              }}
            >
              {t('nodes.enroll.action')}
            </Button>
          </Space>
        }
      >
        <Table
          rowKey="node_id"
          columns={nodeColumns}
          dataSource={nodesQuery.data ?? []}
          loading={nodesQuery.isLoading}
          pagination={tablePagination(t)}
        />
      </Card>

      <Card title={t('nodes.enroll.tokens')} style={{ marginTop: 16 }}>
        <Table
          rowKey="id"
          pagination={tablePagination(t)}
          columns={tokenColumns}
          dataSource={tokensQuery.data ?? []}
          loading={tokensQuery.isLoading}
        />
      </Card>

      <EnrollModal
        open={enrollOpen}
        result={enrollResult}
        onClose={() => setEnrollOpen(false)}
        onCreated={(result) => {
          setEnrollResult({ token: result.plain_token, install: result.install_command })
          void queryClient.invalidateQueries({ queryKey: ['enroll-tokens'] })
        }}
      />

      <Modal
        title={t('nodes.uninstall')}
        open={uninstallOpen}
        footer={null}
        onCancel={() => setUninstallOpen(false)}
      >
        <Paragraph type="secondary">{t('nodes.uninstallHint')}</Paragraph>
        <Paragraph copyable style={{ fontFamily: 'monospace', whiteSpace: 'pre-wrap' }}>
          {uninstallCmd}
        </Paragraph>
      </Modal>
    </Space>
  )
}

/** 接入向导:第一步填节点名与平台;成功后展示一次性令牌与安装命令。 */
function EnrollModal({
  open,
  result,
  onClose,
  onCreated,
}: {
  open: boolean
  result: { token: string; install: string } | null
  onClose: () => void
  onCreated: (r: Awaited<ReturnType<typeof createEnrollToken>>) => void
}) {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const [form] = Form.useForm<{ node_name: string; platform: string }>()
  const [submitting, setSubmitting] = useState(false)

  const submit = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      const created = await createEnrollToken(values.node_name, values.platform)
      onCreated(created)
      form.resetFields()
    } catch (error) {
      message.error(apiErrorMessage(error, t('nodes.enroll.createFailed')))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title={t('nodes.enroll.action')}
      open={open}
      onCancel={onClose}
      footer={
        result ? (
          <Button type="primary" onClick={onClose}>
            {t('common.done')}
          </Button>
        ) : (
          <Space>
            <Button onClick={onClose}>{t('common.cancel')}</Button>
            <Button type="primary" loading={submitting} onClick={() => void submit()}>
              {t('nodes.enroll.generate')}
            </Button>
          </Space>
        )
      }
    >
      {result ? (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <div>
            <Text strong>{t('nodes.enroll.plainToken')}</Text>
            <Paragraph type="secondary">{t('nodes.enroll.plainTokenHint')}</Paragraph>
            <Paragraph copyable style={{ fontFamily: 'monospace' }}>
              {result.token}
            </Paragraph>
          </div>
          <div>
            <Text strong>{t('nodes.enroll.installCommand')}</Text>
            <Paragraph type="secondary">{t('nodes.enroll.installHint')}</Paragraph>
            <Paragraph copyable style={{ fontFamily: 'monospace', whiteSpace: 'pre-wrap' }}>
              {result.install}
            </Paragraph>
          </div>
        </Space>
      ) : (
        <Form form={form} layout="vertical" initialValues={{ platform: 'linux-amd64' }}>
          <Form.Item
            name="node_name"
            label={t('nodes.enroll.nodeName')}
            rules={[{ required: true }]}
            extra={t('nodes.enroll.nodeNameHint')}
          >
            <Input maxLength={128} placeholder="edge-hk-01" />
          </Form.Item>
          <Form.Item name="platform" label={t('nodes.field.platform')} rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'linux-amd64', label: 'linux-amd64' },
                { value: 'linux-arm64', label: 'linux-arm64' },
              ]}
            />
          </Form.Item>
        </Form>
      )}
    </Modal>
  )
}
