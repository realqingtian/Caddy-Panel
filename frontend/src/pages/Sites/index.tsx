import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Card, Popconfirm, Space, Switch, Table, Tag, type TableProps } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import { deleteSite, listSites, setSiteEnabled, type Site } from '@/api/sites'
import { SiteForm } from '@/components/SiteForm'

/** 站点管理页:列表 + 新建/编辑 Drawer + 启停 + 删除(FR-301~307)。 */
export default function SitesPage() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const queryClient = useQueryClient()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Site | null>(null)

  const sitesQuery = useQuery({ queryKey: ['sites'], queryFn: listSites })

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['sites'] })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) => setSiteEnabled(id, enabled),
    onSuccess: (_data, variables) => {
      void message.success(
        variables.enabled ? t('sites.enabledSuccess') : t('sites.disabledSuccess'),
      )
      invalidate()
    },
    onError: (error) => message.error(apiErrorMessage(error, t('sites.saveFailed'))),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteSite(id),
    onSuccess: () => {
      void message.success(t('sites.deletedSuccess'))
      invalidate()
    },
    onError: (error) => message.error(apiErrorMessage(error, t('sites.saveFailed'))),
  })

  const columns: TableProps<Site>['columns'] = [
    { title: t('sites.field.name'), dataIndex: 'name', key: 'name' },
    {
      title: t('sites.field.type'),
      dataIndex: 'type',
      key: 'type',
      width: 110,
      render: (type: Site['type']) => <Tag>{t(`sites.type.${type}`)}</Tag>,
    },
    { title: t('sites.field.domains'), dataIndex: 'domains', key: 'domains', ellipsis: true },
    {
      title: t('sites.field.enabled'),
      dataIndex: 'enabled',
      key: 'enabled',
      width: 90,
      render: (_: boolean, record: Site) => (
        <Switch
          checked={record.enabled}
          loading={toggleMutation.isPending && toggleMutation.variables?.id === record.id}
          onChange={(checked) => toggleMutation.mutate({ id: record.id, enabled: checked })}
        />
      ),
    },
    {
      title: t('common.actions'),
      key: 'actions',
      width: 160,
      render: (_: unknown, record: Site) => (
        <Space>
          <Button
            type="link"
            size="small"
            onClick={() => {
              setEditing(record)
              setFormOpen(true)
            }}
          >
            {t('common.edit')}
          </Button>
          <Popconfirm
            title={t('sites.deleteConfirm')}
            onConfirm={() => deleteMutation.mutate(record.id)}
          >
            <Button type="link" size="small" danger>
              {t('common.delete')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={t('sites.title')}
      extra={
        <Button
          type="primary"
          onClick={() => {
            setEditing(null)
            setFormOpen(true)
          }}
        >
          {t('sites.action.new')}
        </Button>
      }
    >
      <Table<Site>
        rowKey="id"
        loading={sitesQuery.isLoading}
        dataSource={sitesQuery.data ?? []}
        columns={columns}
        pagination={false}
      />
      <SiteForm
        open={formOpen}
        site={editing}
        onClose={() => setFormOpen(false)}
        onSaved={invalidate}
      />
    </Card>
  )
}
