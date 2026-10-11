import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'

import {
  Button,
  Card,
  Drawer,
  Form,
  Input,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'

import { useTranslation } from 'react-i18next'

import {
  createUser,
  deleteUser,
  listUserSessions,
  listUsers,
  revokeUserSession,
  updateUser,
} from '@/api/users'
import type { SessionView, UserView } from '@/api/auth'
import { UserAvatar } from '@/components/UserAvatar'
import { apiErrorMessage } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { App } from 'antd'

/** 用户管理页(admin):CRUD + 会话排查(设计 §8)。 */
export default function UsersPage() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const me = useAuthStore()

  const usersQuery = useQuery({ queryKey: ['users'], queryFn: listUsers })
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [editing, setEditing] = useState<UserView | null>(null)
  const [sessionsOf, setSessionsOf] = useState<UserView | null>(null)

  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['users'] })

  const openCreate = () => {
    setEditing(null)
    setDrawerOpen(true)
  }
  const openEdit = (user: UserView) => {
    setEditing(user)
    setDrawerOpen(true)
  }

  const onDelete = useCallback(
    async (user: UserView) => {
      try {
        await deleteUser(user.id)
        void message.success(t('users.msg.deleted'))
        refresh()
      } catch (error) {
        message.error(apiErrorMessage(error, t('users.msg.deleted')))
      }
    },
    [message, t, refresh],
  )

  const toggleDisable = useCallback(
    async (record: UserView, disabled: boolean) => {
      try {
        await updateUser(record.id, { disabled })
        refresh()
      } catch (error) {
        message.error(apiErrorMessage(error, t('users.msg.saved')))
      }
    },
    [message, t, refresh],
  )

  const columns: ColumnsType<UserView> = useMemo(
    () => [
      {
        title: t('users.col.username'),
        dataIndex: 'username',
        render: (_, record) => (
          <Space>
            <UserAvatar
              username={record.username}
              nickname={record.nickname}
              email={record.email}
              avatarURL={record.avatar_url}
              size={26}
            />
            <span style={{ fontWeight: 600 }}>{record.username}</span>
          </Space>
        ),
      },
      { title: t('users.col.nickname'), dataIndex: 'nickname', render: (v) => v || '—' },
      { title: t('users.col.email'), dataIndex: 'email', render: (v) => v || '—' },
      {
        title: t('users.col.role'),
        dataIndex: 'role',
        render: (role: string) => (
          <Tag color={role === 'admin' ? 'gold' : role === 'editor' ? 'blue' : 'default'}>
            {t(`users.role.${role}`)}
          </Tag>
        ),
      },
      {
        title: t('users.col.status'),
        dataIndex: 'disabled',
        render: (disabled: boolean) =>
          disabled ? (
            <Tag color="error">{t('users.status.disabled')}</Tag>
          ) : (
            <Tag color="success">{t('users.status.normal')}</Tag>
          ),
      },
      {
        title: t('users.col.twofa'),
        dataIndex: 'totp_enabled',
        render: (on: boolean) => (on ? <Tag color="processing">TOTP</Tag> : '—'),
      },
      { title: t('users.col.sessions'), dataIndex: 'session_count' },
      { title: t('users.col.lastActive'), dataIndex: 'last_active_at', render: (v) => v || '—' },
      {
        title: t('common.actions'),
        key: 'actions',
        render: (_, record) => (
          <Space size="small" wrap>
            <Button
              size="small"
              onClick={() => openEdit(record)}
              disabled={record.id === me.userID}
            >
              {t('common.edit')}
            </Button>
            <Button size="small" onClick={() => setSessionsOf(record)}>
              {t('users.action.sessions')} ({record.session_count})
            </Button>
            {record.id !== me.userID && (
              <>
                {record.disabled ? (
                  <Button size="small" onClick={() => void toggleDisable(record, false)}>
                    {t('users.action.enable')}
                  </Button>
                ) : (
                  <Popconfirm
                    title={t('users.confirm.disable', { name: record.username })}
                    onConfirm={() => void toggleDisable(record, true)}
                  >
                    <Button size="small" danger>
                      {t('users.action.disable')}
                    </Button>
                  </Popconfirm>
                )}
                <Popconfirm
                  title={t('users.confirm.delete', { name: record.username })}
                  onConfirm={() => void onDelete(record)}
                >
                  <Button size="small" danger type="link">
                    {t('common.delete')}
                  </Button>
                </Popconfirm>
              </>
            )}
          </Space>
        ),
      },
    ],
    [t, me.userID, onDelete, toggleDisable],
  )

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card
        title={t('users.title')}
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t('users.create')}
          </Button>
        }
      >
        <Table<UserView>
          rowKey="id"
          columns={columns}
          dataSource={usersQuery.data ?? []}
          loading={usersQuery.isLoading}
          pagination={false}
        />
      </Card>

      <UserDrawer
        open={drawerOpen}
        editing={editing}
        onClose={() => setDrawerOpen(false)}
        onSaved={() => {
          setDrawerOpen(false)
          refresh()
        }}
      />

      <SessionsDrawer user={sessionsOf} onClose={() => setSessionsOf(null)} onChanged={refresh} />
    </Space>
  )
}

/** 新建/编辑抽屉:创建含初始密码,编辑含角色/禁用/重置密码(保护规则错误由后端透出)。 */
function UserDrawer({
  open,
  editing,
  onClose,
  onSaved,
}: {
  open: boolean
  editing: UserView | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [pending, setPending] = useState(false)

  const onSubmit = async () => {
    const values = await form.validateFields()
    setPending(true)
    try {
      if (editing) {
        await updateUser(editing.id, {
          nickname: values.nickname,
          email: values.email,
          role: values.role,
          new_password: values.new_password || undefined,
        })
        void message.success(t('users.msg.saved'))
      } else {
        await createUser({
          username: values.username,
          password: values.initial_password,
          nickname: values.nickname,
          email: values.email,
          role: values.role,
        })
        void message.success(t('users.msg.created'))
      }
      onSaved()
    } catch (error) {
      message.error(apiErrorMessage(error, t('users.msg.saved')))
    } finally {
      setPending(false)
    }
  }

  return (
    <Drawer
      title={editing ? t('users.edit') : t('users.createTitle')}
      open={open}
      onClose={onClose}
      width={480}
      destroyOnHidden
      footer={
        <Space style={{ float: 'right' }}>
          <Button onClick={onClose}>{t('common.close')}</Button>
          <Button type="primary" loading={pending} onClick={() => void onSubmit()}>
            {t('common.save')}
          </Button>
        </Space>
      }
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={
          editing
            ? {
                username: editing.username,
                nickname: editing.nickname,
                email: editing.email,
                role: editing.role,
              }
            : { role: 'editor', must_change_password: true }
        }
      >
        <Form.Item
          name="username"
          label={t('users.field.username')}
          rules={[{ required: true, min: 3, max: 64 }]}
        >
          <Input disabled={!!editing} autoComplete="off" />
        </Form.Item>
        {!editing && (
          <Form.Item
            name="initial_password"
            label={t('users.field.initialPassword')}
            rules={[{ required: true, min: 8 }]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        )}
        {editing && (
          <Form.Item name="new_password" label={t('users.field.newPassword')} rules={[{ min: 8 }]}>
            <Input.Password
              autoComplete="new-password"
              placeholder={t('users.field.newPassword')}
            />
          </Form.Item>
        )}
        <Form.Item name="nickname" label={t('users.field.nickname')}>
          <Input />
        </Form.Item>
        <Form.Item
          name="email"
          label={t('users.field.email')}
          rules={[{ type: 'email', message: t('profile.field.email') }]}
        >
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="role" label={t('users.field.role')} rules={[{ required: true }]}>
          <Select
            options={[
              { value: 'admin', label: t('users.role.admin') },
              { value: 'editor', label: t('users.role.editor') },
              { value: 'viewer', label: t('users.role.viewer') },
            ]}
            disabled={!!editing && editing.role === 'admin'}
          />
        </Form.Item>
        {!editing && (
          <Form.Item
            name="must_change_password"
            label={t('users.field.mustChange')}
            valuePropName="checked"
          >
            <Checkbox />
          </Form.Item>
        )}
      </Form>
    </Drawer>
  )
}

import { Checkbox } from 'antd'

/** 会话排查抽屉:查看/强制下线(admin)。 */
function SessionsDrawer({
  user,
  onClose,
  onChanged,
}: {
  user: UserView | null
  onClose: () => void
  onChanged: () => void
}) {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const [sessions, setSessions] = useState<SessionView[] | null>(null)

  const load = async (target: UserView) => {
    try {
      setSessions(await listUserSessions(target.id))
    } catch (error) {
      message.error(apiErrorMessage(error, t('users.sessionsTitle', { name: target.username })))
    }
  }

  const kick = async (sid: string) => {
    if (!user) return
    try {
      await revokeUserSession(user.id, sid)
      await load(user)
      onChanged()
    } catch (error) {
      message.error(apiErrorMessage(error, t('users.action.kick')))
    }
  }

  return (
    <Drawer
      title={user ? t('users.sessionsTitle', { name: user.username }) : ''}
      open={!!user}
      onClose={() => {
        setSessions(null)
        onClose()
      }}
      width={560}
      afterOpenChange={(visible) => visible && user && void load(user)}
    >
      {sessions && (
        <Table<SessionView>
          rowKey="sid"
          size="small"
          pagination={false}
          dataSource={sessions}
          columns={[
            {
              title: t('profile.sessions.device'),
              dataIndex: 'device',
              render: (device: string, record) => (
                <Space>
                  <span>{device}</span>
                  {record.current && <Tag color="processing">{t('profile.sessions.current')}</Tag>}
                  {record.revoked && <Tag>{t('terminal.closed')}</Tag>}
                </Space>
              ),
            },
            { title: t('profile.sessions.ip'), dataIndex: 'ip' },
            { title: t('profile.sessions.lastSeen'), dataIndex: 'last_seen_at' },
            {
              title: t('common.actions'),
              key: 'actions',
              render: (_, record) =>
                record.revoked ? null : (
                  <Popconfirm
                    title={t('users.confirm.kick')}
                    onConfirm={() => void kick(record.sid)}
                  >
                    <Button size="small" danger>
                      {t('users.action.kick')}
                    </Button>
                  </Popconfirm>
                ),
            },
          ]}
        />
      )}
      {user && (
        <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
          {t('users.col.username')}: {user.username}
        </Typography.Paragraph>
      )}
    </Drawer>
  )
}
