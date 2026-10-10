import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  App,
  Button,
  Card,
  Modal,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  type TableProps,
} from 'antd'
import { useState } from 'react'
import { Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import {
  deleteSite,
  getSiteCertificate,
  listSites,
  setSiteEnabled,
  type Site,
  type SiteCertStatus,
} from '@/api/sites'
import { SiteForm } from '@/components/SiteForm'

/** 站点管理页:列表 + 新建/编辑 Drawer + 启停 + 删除(FR-301~307)。 */
export default function SitesPage() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const queryClient = useQueryClient()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Site | null>(null)
  const [certView, setCertView] = useState<{ site: Site; status: SiteCertStatus } | null>(null)

  const showCertificate = async (site: Site) => {
    const status = await getSiteCertificate(site.id)
    setCertView({ site, status })
  }

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
          <Button type="link" size="small" onClick={() => void showCertificate(record)}>
            {t('sites.certificate.action')}
          </Button>
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
      <Modal
        open={certView !== null}
        title={t('sites.certificate.title', { name: certView?.site.name ?? '' })}
        footer={null}
        onCancel={() => setCertView(null)}
      >
        {certView && (
          <div>
            {certView.status.domains.map((d) => (
              <div key={d.domain} style={{ marginBottom: 8 }}>
                <Space>
                  <Typography.Text strong>{d.domain}</Typography.Text>
                  {d.found ? (
                    <Tag color="green">{t('sites.certificate.issued')}</Tag>
                  ) : (
                    <Tag>{t('sites.certificate.notIssued')}</Tag>
                  )}
                </Space>
                {d.found && (
                  <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12 }}>
                    {t('sites.certificate.expires')}: {d.not_after || '—'}
                  </Typography.Text>
                )}
              </div>
            ))}
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('sites.certificate.hint')}
            </Typography.Text>
          </div>
        )}
      </Modal>
      <SiteForm
        open={formOpen}
        site={editing}
        onClose={() => setFormOpen(false)}
        onSaved={invalidate}
      />
    </Card>
  )
}
