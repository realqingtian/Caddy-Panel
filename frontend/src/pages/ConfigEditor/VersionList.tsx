import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  App,
  Button,
  Card,
  Dropdown,
  Empty,
  Input,
  Modal,
  Pagination,
  Segmented,
  Space,
  Tag,
  Typography,
  theme,
} from 'antd'
import { MoreOutlined } from '@ant-design/icons'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { apiErrorMessage } from '@/api/auth'
import { rollbackTo, updateVersionNotes, type ConfigVersion } from '@/api/config'
import { useLocaleTag } from '@/hooks/useLocale'

type StatusFilter = 'all' | 'applied' | 'failed'

const pageSizeDefault = 10

interface VersionListProps {
  versions: ConfigVersion[]
  loading: boolean
  /** 回滚成功后由父组件清对比态并刷新。 */
  onRolledBack: () => void
  /** 当前用户角色(备注编辑仅 admin,logux-design §8)。 */
  role: string | null
  /** 发起与当前配置的对比(父组件切换预览区到 Diff)。 */
  onDiff: (version: ConfigVersion) => void
}

/** 版本历史(logux-design §3):状态过滤默认"已生效"/分页/回滚行尾直出/对比与备注收"···"。 */
export function VersionList({ versions, loading, onRolledBack, role, onDiff }: VersionListProps) {
  const { t } = useTranslation()
  const localeTag = useLocaleTag()
  const { message, modal } = App.useApp()
  const { token } = theme.useToken()
  const queryClient = useQueryClient()

  const [statusFilter, setStatusFilter] = useState<StatusFilter>('applied')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(pageSizeDefault)
  const [notesTarget, setNotesTarget] = useState<ConfigVersion | null>(null)

  const filtered = useMemo(() => {
    if (statusFilter === 'all') return versions
    return versions.filter((v) => v.status === statusFilter)
  }, [versions, statusFilter])

  const pageData = filtered.slice((page - 1) * pageSize, page * pageSize)

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['versions'] })
    void queryClient.invalidateQueries({ queryKey: ['caddyfile'] })
  }

  const rollbackMutation = useMutation({
    mutationFn: (id: number) => rollbackTo(id),
    onSuccess: () => {
      void message.success(t('config.rollbackSuccess'))
      onRolledBack()
      invalidate()
    },
    onError: (error) => message.error(apiErrorMessage(error, t('config.rollbackFailed'))),
  })

  const notesMutation = useMutation({
    mutationFn: ({ id, notes }: { id: number; notes: string }) => updateVersionNotes(id, notes),
    onSuccess: () => {
      void message.success(t('config.notesSaved'))
      setNotesTarget(null)
      void queryClient.invalidateQueries({ queryKey: ['versions'] })
    },
    onError: (error) => message.error(apiErrorMessage(error, t('config.notesSaveFailed'))),
  })

  // 回滚确认(logux-design §3):保留站点影响警告(D11),新增片段覆盖警告(§7)。
  const confirmRollback = (version: ConfigVersion) => {
    modal.confirm({
      title: t('config.rollbackConfirmTitle', { id: version.id }),
      content: (
        <Typography.Paragraph style={{ marginBottom: 0 }}>
          {t('config.rollbackConfirmSites')}
          <br />
          {t('config.rollbackConfirmSnippets')}
        </Typography.Paragraph>
      ),
      okText: t('config.action.rollback'),
      okButtonProps: { danger: true, loading: rollbackMutation.isPending },
      onOk: () => rollbackMutation.mutateAsync(version.id),
    })
  }

  const pageSizeOptions = (
    <Segmented
      value={statusFilter}
      onChange={(v) => {
        setStatusFilter(v as StatusFilter)
        setPage(1)
      }}
      options={[
        { value: 'applied', label: t('config.filter.applied') },
        { value: 'failed', label: t('config.filter.failed') },
        { value: 'all', label: t('config.filter.all') },
      ]}
    />
  )

  return (
    <Card title={t('config.versionsTitle')} extra={pageSizeOptions}>
      <Space direction="vertical" style={{ width: '100%' }} size={8}>
        {pageData.length === 0 && !loading && <Empty description={t('config.noVersions')} />}
        {pageData.map((version) => (
          <VersionCard
            key={version.id}
            version={version}
            localeTag={localeTag}
            borderColor={token.colorBorderSecondary}
            rolling={rollbackMutation.isPending && rollbackMutation.variables === version.id}
            canEditNotes={role === 'admin'}
            onRollback={() => confirmRollback(version)}
            onDiff={() => onDiff(version)}
            onEditNotes={() => setNotesTarget(version)}
          />
        ))}
        {filtered.length > pageSize && (
          <Pagination
            style={{ textAlign: 'right', marginBottom: 0 }}
            current={page}
            pageSize={pageSize}
            total={filtered.length}
            showSizeChanger
            pageSizeOptions={[10, 20, 50]}
            showTotal={(total) => t('common.tableTotal', { total })}
            onChange={(p, ps) => {
              setPage(p)
              setPageSize(ps)
            }}
          />
        )}
      </Space>
      <NotesEditModal
        target={notesTarget}
        saving={notesMutation.isPending}
        onClose={() => setNotesTarget(null)}
        onSave={(notes) => notesMutation.mutate({ id: notesTarget!.id, notes })}
      />
    </Card>
  )
}

interface VersionCardProps {
  version: ConfigVersion
  localeTag: string
  borderColor: string
  rolling: boolean
  canEditNotes: boolean
  onRollback: () => void
  onDiff: () => void
  onEditNotes: () => void
}

/** 单个版本卡片:失败原因默认折叠(最多两行),对比/备注收"···",回滚行尾直出。 */
function VersionCard({
  version,
  localeTag,
  borderColor,
  rolling,
  canEditNotes,
  onRollback,
  onDiff,
  onEditNotes,
}: VersionCardProps) {
  const { t } = useTranslation()
  const applied = version.status === 'applied'
  return (
    <Card
      size="small"
      style={{ borderColor }}
      styles={{ body: { paddingInline: 12, paddingBlock: 8 } }}
      title={
        <Space size={6}>
          <span>#{version.id}</span>
          {applied ? (
            <Tag color="green" style={{ marginInlineEnd: 0 }}>
              {t('config.status.applied')}
            </Tag>
          ) : (
            <Tag color="red" style={{ marginInlineEnd: 0 }}>
              {t('config.status.failed')}
            </Tag>
          )}
        </Space>
      }
      extra={
        <Space size={4}>
          <Button
            size="small"
            type="primary"
            ghost
            disabled={!applied}
            loading={rolling}
            onClick={onRollback}
          >
            {t('config.action.rollback')}
          </Button>
          <Dropdown
            menu={{
              items: [
                { key: 'diff', label: t('config.action.diff') },
                ...(canEditNotes ? [{ key: 'notes', label: t('config.action.editNotes') }] : []),
              ],
              onClick: ({ key }) => {
                if (key === 'diff') onDiff()
                if (key === 'notes') onEditNotes()
              },
            }}
          >
            <Button size="small" icon={<MoreOutlined />} />
          </Dropdown>
        </Space>
      }
    >
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        {new Date(version.created_at).toLocaleString(localeTag)}
      </Typography.Text>
      {version.notes && (
        <Typography.Paragraph
          style={{ fontSize: 12, margin: '4px 0 0', color: 'inherit' }}
          ellipsis={{
            rows: 2,
            expandable: 'collapsible',
            symbol: (expanded) => (expanded ? t('common.collapse') : t('common.expand')),
          }}
        >
          {version.notes}
        </Typography.Paragraph>
      )}
      {!applied && version.error_msg && (
        <Typography.Paragraph
          type="danger"
          style={{ fontSize: 12, marginTop: 4, marginBottom: 0 }}
          ellipsis={{
            rows: 2,
            expandable: 'collapsible',
            symbol: (expanded) => (expanded ? t('common.collapse') : t('common.expand')),
          }}
        >
          {version.error_msg}
        </Typography.Paragraph>
      )}
    </Card>
  )
}

interface NotesEditModalProps {
  target: ConfigVersion | null
  saving: boolean
  onClose: () => void
  onSave: (notes: string) => void
}

/** 版本备注编辑(logux-design §8):单行输入,空值即清空。 */
function NotesEditModal({ target, saving, onClose, onSave }: NotesEditModalProps) {
  const { t } = useTranslation()
  const [notes, setNotes] = useState('')
  // 打开时以目标版本当前备注初始化(渲染期间条件赋值,避免 effect 级联)。
  const [initializedFor, setInitializedFor] = useState<number | null>(null)
  if (target && initializedFor !== target.id) {
    setInitializedFor(target.id)
    setNotes(target.notes ?? '')
  }
  if (!target && initializedFor !== null) {
    setInitializedFor(null)
  }

  return (
    <Modal
      title={t('config.notesModalTitle', { id: target?.id ?? '' })}
      open={!!target}
      onCancel={onClose}
      onOk={() => onSave(notes)}
      okText={t('common.save')}
      confirmLoading={saving}
      destroyOnHidden
    >
      <Input
        value={notes}
        maxLength={200}
        showCount
        placeholder={t('config.notesPlaceholder')}
        onChange={(e) => setNotes(e.target.value)}
        onPressEnter={() => onSave(notes)}
      />
    </Modal>
  )
}
