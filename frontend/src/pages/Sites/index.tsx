import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert,
  App,
  Button,
  Card,
  Input,
  Modal,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  Upload,
  type TableProps,
} from 'antd'
import { useRef, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import {
  applyImport,
  deleteSite,
  getSiteCertificate,
  listSites,
  previewImport,
  setSiteEnabled,
  type ImportedSite,
  type ImportPreview,
  type Site,
  type SiteCertStatus,
} from '@/api/sites'
import { SiteForm } from '@/components/SiteForm'
import { uploadCertificate } from '@/api/sites'
import { UploadOutlined } from '@ant-design/icons'
import type { UploadFile } from 'antd'

/** 站点管理页:列表 + 新建/编辑 Drawer + 启停 + 删除(FR-301~307)。 */
export default function SitesPage() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const queryClient = useQueryClient()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Site | null>(null)
  const [certView, setCertView] = useState<{ site: Site; status: SiteCertStatus } | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [importContent, setImportContent] = useState('')
  const [importResult, setImportResult] = useState<ImportPreview | null>(null)
  const [importBusy, setImportBusy] = useState(false)
  const [importDone, setImportDone] = useState<{ created: string[]; skipped: string[] } | null>(
    null,
  )

  const runPreview = async () => {
    setImportBusy(true)
    try {
      setImportResult(await previewImport(importContent))
    } catch (error) {
      message.error(apiErrorMessage(error, t('sites.import.parseFailed')))
    } finally {
      setImportBusy(false)
    }
  }

  const runApply = async () => {
    setImportBusy(true)
    try {
      const result = await applyImport(importContent)
      setImportDone(result)
      setImportResult(null)
      setImportContent('')
      invalidate()
    } catch (error) {
      message.error(apiErrorMessage(error, t('sites.import.failed')))
    } finally {
      setImportBusy(false)
    }
  }

  const uploadCertRef = useRef<File | null>(null)

  const doUploadCertificate = async (keyFile: File) => {
    if (!certView || !uploadCertRef.current) {
      message.warning(t('sites.certificate.selectCertFirst'))
      return
    }
    try {
      await uploadCertificate(certView.site.id, uploadCertRef.current, keyFile)
      void message.success(t('sites.certificate.uploadSuccess'))
      invalidate()
    } catch (error) {
      message.error(apiErrorMessage(error, t('sites.certificate.uploadFailed')))
    }
  }

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
      title: t('sites.field.node'),
      dataIndex: 'node_id',
      key: 'node',
      width: 120,
      render: (v: string) => <Tag>{v || 'local'}</Tag>,
    },
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
        <Space>
          <Button
            onClick={() => {
              setImportOpen(true)
              setImportResult(null)
              setImportDone(null)
            }}
          >
            {t('sites.import.action')}
          </Button>
          <Button
            type="primary"
            onClick={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            {t('sites.action.new')}
          </Button>
        </Space>
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
            <Upload
              accept=".pem,.crt,.cer"
              showUploadList={false}
              beforeUpload={(certFile: UploadFile) => {
                // 两步选择:先证书后私钥(此处取 cert,由自定义 input 补 key)
                uploadCertRef.current = certFile as unknown as File
                return false
              }}
            >
              <Button size="small" icon={<UploadOutlined />}>
                {t('sites.certificate.uploadCert')}
              </Button>
            </Upload>
            <Upload
              accept=".pem,.key"
              showUploadList={false}
              beforeUpload={(keyFile: UploadFile) => {
                void doUploadCertificate(keyFile as unknown as File)
                return false
              }}
            >
              <Button size="small" icon={<UploadOutlined />}>
                {t('sites.certificate.uploadKey')}
              </Button>
            </Upload>
            <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12 }}>
              {t('sites.certificate.uploadHint')}
            </Typography.Text>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('sites.certificate.hint')}
            </Typography.Text>
          </div>
        )}
      </Modal>
      <Modal
        open={importOpen}
        title={t('sites.import.title')}
        width={720}
        onCancel={() => setImportOpen(false)}
        footer={
          importDone ? (
            <Button onClick={() => setImportOpen(false)}>{t('common.close')}</Button>
          ) : importResult ? (
            <Space>
              <Button onClick={() => setImportResult(null)}>{t('common.reset')}</Button>
              <Button type="primary" loading={importBusy} onClick={() => void runApply()}>
                {t('sites.import.confirm')}
              </Button>
            </Space>
          ) : (
            <Button
              type="primary"
              loading={importBusy}
              disabled={!importContent.trim()}
              onClick={() => void runPreview()}
            >
              {t('sites.import.preview')}
            </Button>
          )
        }
      >
        {importDone ? (
          <div>
            <Alert
              type="success"
              showIcon
              message={t('sites.import.done')}
              style={{ marginBottom: 12 }}
            />
            <Typography.Paragraph>
              {t('sites.import.created')}: {importDone.created.join(', ') || '—'}
            </Typography.Paragraph>
            <Typography.Paragraph>
              {t('sites.import.skipped')}: {importDone.skipped.join(', ') || '—'}
            </Typography.Paragraph>
            <Typography.Text type="secondary">{t('sites.import.disabledHint')}</Typography.Text>
          </div>
        ) : importResult ? (
          <div>
            {importResult.global_extra && (
              <Alert
                type="info"
                showIcon
                message={t('sites.import.globalExtra')}
                description={
                  <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>
                    {importResult.global_extra}
                  </pre>
                }
                style={{ marginBottom: 12 }}
              />
            )}
            {importResult.sites.map((site: ImportedSite) => (
              <Card
                key={site.name}
                size="small"
                style={{ marginBottom: 8 }}
                title={`${site.name}(${t('sites.type.' + site.type)})`}
              >
                <Typography.Text>{site.domains}</Typography.Text>
                {site.upstreams && (
                  <Typography.Text type="secondary"> → {site.upstreams}</Typography.Text>
                )}
                {site.warnings?.map((warning: string) => (
                  <Alert key={warning} type="warning" message={warning} style={{ marginTop: 8 }} />
                ))}
              </Card>
            ))}
            {importResult.sites.length === 0 && (
              <Alert type="warning" showIcon message={t('sites.import.noSites')} />
            )}
          </div>
        ) : (
          <div>
            <Alert
              type="info"
              showIcon
              message={t('sites.import.previewHint')}
              style={{ marginBottom: 12 }}
            />
            <Input.TextArea
              rows={14}
              value={importContent}
              onChange={(e: ChangeEvent<HTMLTextAreaElement>) => setImportContent(e.target.value)}
              placeholder={t('sites.import.placeholder')}
            />
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
