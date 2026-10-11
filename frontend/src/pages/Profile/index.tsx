import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import {
  App,
  Button,
  Card,
  Modal,
  Input,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
} from 'antd'
import {
  DeleteOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  UploadOutlined,
} from '@ant-design/icons'
import { useTranslation } from 'react-i18next'

import {
  apiErrorMessage,
  changePassword,
  disable2FA,
  enable2FA,
  getProfile,
  listMySessions,
  patchProfile,
  regenerateRecoveryCodes,
  removeAvatar,
  revokeMySession,
  revokeOtherSessions,
  setup2FA,
  uploadAvatar,
  type SessionView,
} from '@/api/auth'
import { UserAvatar } from '@/components/UserAvatar'
import { useAuthStore } from '@/stores/auth'

/** 个人设置页:资料/改密/两步验证/登录设备(设计 §8.1),全部角色可用。 */
export default function ProfilePage() {
  const { t } = useTranslation()
  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card title={t('profile.section.profile')}>
        <ProfileSection />
      </Card>
      <Card title={t('profile.section.password')}>
        <PasswordSection />
      </Card>
      <Card title={t('profile.section.twofa')}>
        <TwoFASection />
      </Card>
      <Card title={t('profile.section.sessions')}>
        <SessionsSection />
      </Card>
    </Space>
  )
}

/** ① 资料区:头像 + 昵称/邮箱 + 只读身份。 */
function ProfileSection() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const setNickname = useAuthStore((s) => s.setNickname)
  const profileQuery = useQuery({ queryKey: ['profile'], queryFn: getProfile })
  const profile = profileQuery.data
  const [nickname, setNicknameState] = useState<string | null>(null)
  const [email, setEmailState] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  if (profileQuery.isLoading || !profile) {
    return <Typography.Text type="secondary">{t('common.loading')}</Typography.Text>
  }
  const nick = nickname ?? profile.nickname
  const mail = email ?? profile.email

  const save = async () => {
    setSaving(true)
    try {
      await patchProfile(nick, mail)
      setNickname(nick)
      void message.success(t('profile.profileSaved'))
      void queryClient.invalidateQueries({ queryKey: ['profile'] })
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.profileSaved')))
    } finally {
      setSaving(false)
    }
  }

  const onUpload = async (file: File) => {
    try {
      await uploadAvatar(file)
      void queryClient.invalidateQueries({ queryKey: ['profile'] })
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.avatar.hint')))
    }
    return false
  }

  const onRemove = async () => {
    try {
      await removeAvatar()
      void queryClient.invalidateQueries({ queryKey: ['profile'] })
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.avatar.remove')))
    }
  }

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Space size={24} align="center">
        <UserAvatar
          username={profile.username}
          nickname={profile.nickname}
          email={profile.email}
          avatarURL={profile.avatar_url}
          size={72}
        />
        <Space direction="vertical" size={4}>
          <Upload accept=".png,.jpg,.jpeg,.webp" showUploadList={false} beforeUpload={onUpload}>
            <Button icon={<UploadOutlined />}>{t('profile.avatar.upload')}</Button>
          </Upload>
          {profile.avatar_url && (
            <Button icon={<DeleteOutlined />} size="small" onClick={() => void onRemove()}>
              {t('profile.avatar.remove')}
            </Button>
          )}
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {t('profile.avatar.hint')}
          </Typography.Text>
        </Space>
      </Space>
      <Space direction="vertical" size={8} style={{ width: '100%', maxWidth: 480 }}>
        <Typography.Text>{t('profile.field.username')}</Typography.Text>
        <Input value={profile.username} disabled />
        <Typography.Text>{t('profile.field.role')}</Typography.Text>
        <Input value={t(`users.role.${profile.role}`)} disabled />
        <Typography.Text>{t('profile.field.nickname')}</Typography.Text>
        <Input
          value={nick}
          placeholder={t('profile.field.nicknameHint')}
          onChange={(e) => setNicknameState(e.target.value)}
        />
        <Typography.Text>{t('profile.field.email')}</Typography.Text>
        <Input
          value={mail}
          onChange={(e) => setEmailState(e.target.value)}
          placeholder="admin@example.com"
        />
        <Button type="primary" loading={saving} onClick={() => void save()}>
          {t('common.save')}
        </Button>
      </Space>
    </Space>
  )
}

/** ② 修改密码(同时也是首登强制改密表单的复用形态)。 */
export function PasswordForm({
  requireOld = true,
  onDone,
}: {
  requireOld?: boolean
  onDone?: () => void
}) {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const clearSession = useAuthStore((s) => s.clear)
  const clearMustChangePassword = useAuthStore((s) => s.clearMustChangePassword)
  const [pending, setPending] = useState(false)
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirmPwd, setConfirmPwd] = useState('')

  const submit = async () => {
    if (newPwd.length < 8) {
      message.error(t('profile.pwd.hint'))
      return
    }
    if (newPwd !== confirmPwd) {
      message.error(t('profile.pwd.mismatch'))
      return
    }
    setPending(true)
    try {
      await changePassword(requireOld ? oldPwd : '', newPwd)
      clearMustChangePassword()
      void message.success(t('profile.pwd.done'))
      if (onDone) {
        onDone()
      } else {
        // 全部会话已吊销(含当前):跳转登录。
        setTimeout(() => {
          clearSession()
          window.location.href = '/login'
        }, 800)
      }
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.pwd.hint')))
    } finally {
      setPending(false)
    }
  }

  return (
    <Space direction="vertical" size={8} style={{ width: '100%', maxWidth: 420 }}>
      {requireOld && (
        <Input.Password
          value={oldPwd}
          onChange={(e) => setOldPwd(e.target.value)}
          placeholder={t('profile.field.oldPassword')}
          autoComplete="current-password"
        />
      )}
      <Input.Password
        value={newPwd}
        onChange={(e) => setNewPwd(e.target.value)}
        placeholder={t('profile.field.newPassword')}
        autoComplete="new-password"
      />
      <Input.Password
        value={confirmPwd}
        onChange={(e) => setConfirmPwd(e.target.value)}
        placeholder={t('profile.field.confirmPassword')}
        status={confirmPwd && confirmPwd !== newPwd ? 'error' : undefined}
        autoComplete="new-password"
      />
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        {t('profile.pwd.hint')}
      </Typography.Text>
      <Button
        type="primary"
        loading={pending}
        disabled={!newPwd || newPwd !== confirmPwd}
        onClick={() => void submit()}
      >
        {t('profile.pwd.save')}
      </Button>
    </Space>
  )
}

/** 修改密码卡片。
 * eslint 声明:PasswordForm 为页内复用形态(强制改密弹窗复用同一表单组件)。
 */
function PasswordSection() {
  return <PasswordForm />
}

/** ③ 两步验证:绑定/解绑/恢复码。 */
function TwoFASection() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const profileQuery = useQuery({ queryKey: ['profile'], queryFn: getProfile })
  const profile = profileQuery.data
  const [setupState, setSetupState] = useState<{ secret: string; uri: string } | null>(null)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null)
  const [qrSVG, setQRSVG] = useState<string | null>(null)

  const startSetup = async () => {
    try {
      const setup = await setup2FA()
      setSetupState(setup)
      // 轻量 QR:动态引入(避免首屏体积)。
      const QRCode = (await import('qrcode')).default
      setQRSVG(await QRCode.toString(setup.uri, { type: 'svg', margin: 1 }))
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.twofa.enable')))
    }
  }

  const confirmEnable = async () => {
    if (!setupState) return
    setPending(true)
    try {
      const codes = await enable2FA(setupState.secret, code)
      setRecoveryCodes(codes)
      setSetupState(null)
      void queryClient.invalidateQueries({ queryKey: ['profile'] })
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.twofa.enterCode')))
    } finally {
      setPending(false)
    }
  }

  const onDisable = async () => {
    setPending(true)
    try {
      await disable2FA(password, code)
      setPassword('')
      setCode('')
      void queryClient.invalidateQueries({ queryKey: ['profile'] })
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.twofa.disable')))
    } finally {
      setPending(false)
    }
  }

  const onRegenerate = async () => {
    setPending(true)
    try {
      const codes = await regenerateRecoveryCodes(code)
      setRecoveryCodes(codes)
      setCode('')
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.twofa.regenerate')))
    } finally {
      setPending(false)
    }
  }

  if (profileQuery.isLoading || !profile) {
    return <Typography.Text type="secondary">{t('common.loading')}</Typography.Text>
  }

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      {profile.totp_enabled ? (
        <>
          <Tag color="success">{t('profile.twofa.enabled')}</Tag>
          <Space direction="vertical" size={8} style={{ width: '100%', maxWidth: 420 }}>
            <Input.Password
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={t('profile.field.oldPassword')}
            />
            <Input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder={t('login.code')}
            />
            <Space wrap>
              <Button
                icon={<SafetyCertificateOutlined />}
                onClick={() => void onRegenerate()}
                loading={pending}
              >
                {t('profile.twofa.regenerate')}
              </Button>
              <Popconfirm title={t('profile.twofa.disableHint')} onConfirm={() => void onDisable()}>
                <Button danger>{t('profile.twofa.disable')}</Button>
              </Popconfirm>
            </Space>
          </Space>
        </>
      ) : setupState ? (
        <Space direction="vertical" size={12}>
          {qrSVG && (
            <div
              style={{ background: '#fff', padding: 12, borderRadius: 8 }}
              dangerouslySetInnerHTML={{ __html: qrSVG }}
            />
          )}
          <Typography.Text>{t('profile.twofa.scan')}</Typography.Text>
          <Typography.Text copyable code style={{ fontSize: 12 }}>
            {setupState.secret}
          </Typography.Text>
          <Input
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder={t('profile.twofa.enterCode')}
            style={{ maxWidth: 240 }}
          />
          <Button type="primary" loading={pending} onClick={() => void confirmEnable()}>
            {t('profile.twofa.confirmEnable')}
          </Button>
        </Space>
      ) : (
        <Space direction="vertical" size={8}>
          <Tag>{t('profile.twofa.disabled')}</Tag>
          <Button icon={<SafetyCertificateOutlined />} onClick={() => void startSetup()}>
            {t('profile.twofa.enable')}
          </Button>
        </Space>
      )}

      <Modal
        open={!!recoveryCodes}
        title={t('profile.twofa.codes')}
        footer={
          <Button
            type="primary"
            onClick={() => {
              void navigator.clipboard.writeText((recoveryCodes ?? []).join('\n'))
              void message.success(t('profile.twofa.codesCopy'))
            }}
          >
            {t('profile.twofa.codesCopy')}
          </Button>
        }
        onCancel={() => setRecoveryCodes(null)}
      >
        <ul style={{ columns: 2 }}>
          {(recoveryCodes ?? []).map((code) => (
            <li key={code}>
              <Typography.Text code>{code}</Typography.Text>
            </li>
          ))}
        </ul>
        <Typography.Text type="danger">{t('profile.twofa.codes')}</Typography.Text>
      </Modal>
    </Space>
  )
}

/** ④ 登录设备(会话管理)。 */
function SessionsSection() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const sessionsQuery = useQuery({ queryKey: ['my-sessions'], queryFn: listMySessions })
  const [pending, setPending] = useState(false)

  const revokeOne = async (sid: string) => {
    setPending(true)
    try {
      await revokeMySession(sid)
      void sessionsQuery.refetch()
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.sessions.revoke')))
    } finally {
      setPending(false)
    }
  }

  const revokeOthers = async () => {
    setPending(true)
    try {
      await revokeOtherSessions()
      void sessionsQuery.refetch()
    } catch (error) {
      message.error(apiErrorMessage(error, t('profile.sessions.revokeOthers')))
    } finally {
      setPending(false)
    }
  }

  const sessions = sessionsQuery.data ?? []

  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Button
        icon={<RollbackOutlined />}
        onClick={() => void revokeOthers()}
        loading={pending}
        disabled={sessions.filter((s) => !s.current && !s.revoked).length === 0}
      >
        {t('profile.sessions.revokeOthers')}
      </Button>
      <Table<SessionView>
        rowKey="sid"
        size="small"
        pagination={false}
        loading={sessionsQuery.isLoading}
        dataSource={sessions.filter((s) => !s.revoked)}
        columns={[
          {
            title: t('profile.sessions.device'),
            dataIndex: 'device',
            render: (device: string, record) => (
              <Space>
                <span>{device}</span>
                {record.current && <Tag color="processing">{t('profile.sessions.current')}</Tag>}
              </Space>
            ),
          },
          { title: t('profile.sessions.ip'), dataIndex: 'ip' },
          { title: t('profile.sessions.lastSeen'), dataIndex: 'last_seen_at' },
          { title: t('profile.sessions.createdAt'), dataIndex: 'created_at' },
          {
            title: t('common.actions'),
            key: 'actions',
            render: (_, record) =>
              record.current ? null : (
                <Popconfirm
                  title={t('profile.sessions.confirmRevoke')}
                  onConfirm={() => void revokeOne(record.sid)}
                >
                  <Button size="small" danger>
                    {t('profile.sessions.revoke')}
                  </Button>
                </Popconfirm>
              ),
          },
        ]}
      />
    </Space>
  )
}
