import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  App,
  Button,
  Card,
  Empty,
  Form,
  Input,
  Modal,
  Space,
  Switch,
  Table,
  Typography,
} from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useState } from 'react'
import type { ColumnsType } from 'antd/es/table'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import {
  createSnippet,
  deleteSnippet,
  listSnippets,
  updateSnippet,
  type Snippet,
  type SnippetInput,
} from '@/api/config'
import { CaddyfileEditor } from '@/components/CaddyfileEditor'
import { useLocaleTag } from '@/hooks/useLocale'
import { tablePagination } from '@/lib/tablePagination'

/** 片段管理(logux-design §7):具名 Caddyfile 块,启用片段按创建顺序追加 local 产物尾部。 */
export function SnippetPane() {
  const { t } = useTranslation()
  const localeTag = useLocaleTag()
  const { message, modal } = App.useApp()
  const queryClient = useQueryClient()

  const snippetsQuery = useQuery({ queryKey: ['snippets'], queryFn: listSnippets })
  const [editing, setEditing] = useState<Snippet | null>(null)
  const [creating, setCreating] = useState(false)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['snippets'] })
    void queryClient.invalidateQueries({ queryKey: ['caddyfile'] })
    void queryClient.invalidateQueries({ queryKey: ['versions'] })
  }

  const toggleMutation = useMutation({
    mutationFn: ({ snippet, enabled }: { snippet: Snippet; enabled: boolean }) =>
      updateSnippet(snippet.id, { name: snippet.name, content: snippet.content, enabled }),
    onSuccess: () => {
      void message.success(t('config.snippet.saved'))
      invalidate()
    },
    onError: (error) => message.error(apiErrorMessage(error, t('config.snippet.saveFailed'))),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteSnippet(id),
    onSuccess: () => {
      void message.success(t('config.snippet.deleted'))
      invalidate()
    },
    onError: (error) => message.error(apiErrorMessage(error, t('config.snippet.saveFailed'))),
  })

  // 删除确认:启用片段删除会重下发移除产物块,失败则取消删除(§7)。
  const confirmDelete = (snippet: Snippet) => {
    modal.confirm({
      title: t('config.snippet.deleteConfirmTitle', { name: snippet.name }),
      content: snippet.enabled
        ? t('config.snippet.deleteConfirmEnabled')
        : t('config.snippet.deleteConfirmDisabled'),
      okButtonProps: { danger: true, loading: deleteMutation.isPending },
      onOk: () => deleteMutation.mutateAsync(snippet.id),
    })
  }

  const columns: ColumnsType<Snippet> = [
    {
      title: t('config.snippet.colName'),
      dataIndex: 'name',
      width: 200,
      render: (name: string) => <Typography.Text strong>{name}</Typography.Text>,
    },
    {
      title: t('config.snippet.colEnabled'),
      dataIndex: 'enabled',
      width: 100,
      render: (enabled: boolean, record) => (
        <Switch
          checked={enabled}
          loading={toggleMutation.isPending && toggleMutation.variables?.snippet.id === record.id}
          onChange={(checked) => toggleMutation.mutate({ snippet: record, enabled: checked })}
        />
      ),
    },
    {
      title: t('config.snippet.colUpdatedAt'),
      dataIndex: 'updated_at',
      render: (value: string) => new Date(value).toLocaleString(localeTag),
    },
    {
      title: t('common.actions'),
      key: 'actions',
      width: 120,
      render: (_, record) => (
        <Space>
          <Button size="small" onClick={() => setEditing(record)}>
            {t('common.edit')}
          </Button>
          <Button size="small" danger onClick={() => confirmDelete(record)}>
            {t('common.delete')}
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={t('config.snippet.title')}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
          {t('config.snippet.create')}
        </Button>
      }
    >
      <Alert
        style={{ marginBottom: 16 }}
        type="info"
        showIcon
        message={t('config.snippet.hintTitle')}
        description={t('config.snippet.hintBody')}
      />
      <Table<Snippet>
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={snippetsQuery.data ?? []}
        loading={snippetsQuery.isLoading}
        pagination={tablePagination(t)}
        locale={{ emptyText: <Empty description={t('config.snippet.empty')} /> }}
      />
      <SnippetEditModal
        open={creating || !!editing}
        target={editing}
        onClose={() => {
          setCreating(false)
          setEditing(null)
        }}
        onSaved={() => {
          setCreating(false)
          setEditing(null)
          invalidate()
        }}
      />
    </Card>
  )
}

interface SnippetEditModalProps {
  open: boolean
  target: Snippet | null
  onClose: () => void
  onSaved: () => void
}

/** 片段新建/编辑:名称+内容(Monaco)+启用开关;保存即渲染下发,失败保持弹窗。 */
function SnippetEditModal({ open, target, onClose, onSaved }: SnippetEditModalProps) {
  const { t } = useTranslation()
  const { message, notification } = App.useApp()
  const [form] = Form.useForm<SnippetInput>()
  const [saving, setSaving] = useState(false)

  const mutation = useMutation({
    mutationFn: (input: SnippetInput) =>
      target ? updateSnippet(target.id, input) : createSnippet(input),
    onSuccess: () => {
      void message.success(t('config.snippet.saved'))
      onSaved()
    },
    onError: (error) => {
      notification.error({
        message: t('config.snippet.saveFailed'),
        description: apiErrorMessage(error, t('config.snippet.saveFailed')),
        placement: 'bottomRight',
      })
    },
  })

  const handleOk = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      await mutation.mutateAsync(values)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title={
        target
          ? t('config.snippet.editTitle', { name: target.name })
          : t('config.snippet.createTitle')
      }
      open={open}
      onCancel={onClose}
      onOk={() => void handleOk()}
      okText={t('common.save')}
      confirmLoading={saving}
      width={720}
      destroyOnHidden
    >
      <Form<SnippetInput>
        form={form}
        layout="vertical"
        initialValues={target ?? { name: '', content: '', enabled: true }}
        preserve={false}
      >
        <Space style={{ display: 'flex' }} align="start">
          <Form.Item
            name="name"
            label={t('config.snippet.colName')}
            style={{ flex: '0 0 240px' }}
            rules={[{ required: true, message: t('config.snippet.nameRequired') }]}
          >
            <Input maxLength={64} placeholder="my-snippet" />
          </Form.Item>
          <Form.Item name="enabled" label={t('config.snippet.colEnabled')} valuePropName="checked">
            <Switch />
          </Form.Item>
        </Space>
        <Form.Item
          name="content"
          label={t('config.snippet.colContent')}
          rules={[{ required: true, message: t('config.snippet.contentRequired') }]}
          extra={t('config.snippet.contentExtra')}
        >
          {/* Form.Item 注入 value/onChange,CaddyfileEditor 恰为受控组件签名 */}
          <CaddyfileEditor height="260px" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
