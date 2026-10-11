import { useState, type ReactNode } from 'react'

import {
  ApiOutlined,
  DesktopOutlined,
  GlobalOutlined,
  MoonOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  SunOutlined,
} from '@ant-design/icons'
import { App, Button, Dropdown, Form, Input, Segmented, theme, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'

import { login, login2FA, apiErrorMessage } from '@/api/auth'
import { BrandMark } from '@/components/BrandMark'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type AppLocale, type ThemeMode } from '@/stores/theme'

interface LoginValues {
  username: string
  password: string
}

const themeOptions: { value: ThemeMode; icon: ReactNode }[] = [
  { value: 'light', icon: <SunOutlined /> },
  { value: 'dark', icon: <MoonOutlined /> },
  { value: 'system', icon: <DesktopOutlined /> },
]

/** 登录页:左侧品牌区 + 右侧表单区;调用 POST /api/v1/auth/login;错误消息为后端 i18n 文案(FR-101/103)。 */
export default function LoginPage() {
  const { t } = useTranslation()
  const { token } = theme.useToken()
  const { message } = App.useApp()
  const navigate = useNavigate()
  const setSession = useAuthStore((state) => state.setSession)
  const [submitting, setSubmitting] = useState(false)
  // 两步验证状态:ticket 非空即处于二段验证输入(设计 §5)。
  const [ticket, setTicket] = useState<string | null>(null)

  const completeLogin = (result: {
    token: string
    user_id: number
    username: string
    nickname: string
    role: string
    must_change_password: boolean
  }) => {
    setSession(
      result.token,
      result.user_id,
      result.username,
      result.role,
      result.nickname,
      result.must_change_password,
    )
    navigate('/', { replace: true })
  }

  const onFinish = async (values: LoginValues) => {
    setSubmitting(true)
    try {
      const result = await login(values.username, values.password)
      if (result.require_2fa && result.ticket) {
        setTicket(result.ticket)
        return
      }
      completeLogin(result)
    } catch (error) {
      message.error(apiErrorMessage(error, t('login.failed')))
    } finally {
      setSubmitting(false)
    }
  }

  const onFinish2FA = async (values: { code: string }) => {
    if (!ticket) return
    setSubmitting(true)
    try {
      const result = await login2FA(ticket, values.code)
      completeLogin(result)
    } catch (error) {
      message.error(apiErrorMessage(error, t('login.failed')))
    } finally {
      setSubmitting(false)
    }
  }

  const points = [
    { icon: <ApiOutlined />, title: t('login.pointSitesTitle'), desc: t('login.pointSitesDesc') },
    {
      icon: <SafetyCertificateOutlined />,
      title: t('login.pointCertTitle'),
      desc: t('login.pointCertDesc'),
    },
    {
      icon: <RollbackOutlined />,
      title: t('login.pointRollbackTitle'),
      desc: t('login.pointRollbackDesc'),
    },
  ]

  return (
    <div className="login-page" style={{ background: token.colorBgLayout, color: token.colorText }}>
      <section
        className="login-brand"
        style={{
          borderColor: token.colorSplit,
          backgroundImage: `linear-gradient(${token.colorFillQuaternary} 1px, transparent 1px), linear-gradient(90deg, ${token.colorFillQuaternary} 1px, transparent 1px)`,
        }}
      >
        <div className="login-brand-head">
          <BrandMark color={token.colorPrimary} />
          <span>{t('app.title')}</span>
        </div>
        <div className="login-brand-main">
          <p className="login-brand-tagline">{t('login.brandTagline')}</p>
          <ul className="login-points">
            {points.map((point) => (
              <li key={point.title} className="login-point">
                <span className="login-point-icon" style={{ color: token.colorPrimary }}>
                  {point.icon}
                </span>
                <span>
                  <span className="login-point-title">{point.title}</span>
                  <span className="login-point-desc" style={{ color: token.colorTextSecondary }}>
                    {point.desc}
                  </span>
                </span>
              </li>
            ))}
          </ul>
          <TopologyArt accent={token.colorPrimary} line={token.colorBorder} />
        </div>
      </section>
      <section className="login-pane">
        <PaneToolbar />
        <div className="login-form-box">
          <Typography.Title level={3} style={{ marginBottom: 4 }}>
            {t('login.welcome')}
          </Typography.Title>
          <Typography.Text type="secondary">
            <p style={{ marginBottom: 24 }}>{t('login.welcomeHint')}</p>
          </Typography.Text>
          {ticket ? (
            <Form<{ code: string }> onFinish={onFinish2FA} layout="vertical" requiredMark={false}>
              <Typography.Title level={4} style={{ marginBottom: 12 }}>
                {t('login.twofaTitle')}
              </Typography.Title>
              <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
                {t('login.twofaHint')}
              </Typography.Paragraph>
              <Form.Item name="code" rules={[{ required: true, message: t('login.code') }]}>
                <Input
                  autoFocus
                  inputMode="numeric"
                  maxLength={29}
                  placeholder={t('login.code')}
                  style={{ textAlign: 'center', letterSpacing: 6, fontSize: 16 }}
                />
              </Form.Item>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                {t('login.verify')}
              </Button>
            </Form>
          ) : (
            <Form<LoginValues> onFinish={onFinish} layout="vertical" requiredMark={false}>
              <Form.Item
                name="username"
                label={t('login.username')}
                rules={[{ required: true, message: t('login.usernameRequired') }]}
              >
                <Input autoComplete="username" placeholder={t('login.username')} />
              </Form.Item>
              <Form.Item
                name="password"
                label={t('login.password')}
                rules={[{ required: true, message: t('login.passwordRequired') }]}
              >
                <Input.Password autoComplete="current-password" placeholder={t('login.password')} />
              </Form.Item>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                {t('login.submit')}
              </Button>
            </Form>
          )}
          <Typography.Text
            type="secondary"
            style={{ display: 'block', marginTop: 20, fontSize: 12 }}
          >
            {t('login.firstRunHint')}
          </Typography.Text>
        </div>
      </section>
    </div>
  )
}

/** 品牌区右上角的主题/语言切换,交互与主界面 HeaderActions 保持一致(FR-603/604)。 */
function PaneToolbar() {
  const { t } = useTranslation()
  const mode = useThemeStore((state) => state.mode)
  const setMode = useThemeStore((state) => state.setMode)
  const setLocale = useThemeStore((state) => state.setLocale)

  const languageItems = [
    { key: 'zh-CN', label: t('layout.langZhCN') },
    { key: 'en-US', label: t('layout.langEnUS') },
  ]

  return (
    <div className="login-pane-toolbar">
      <Segmented
        aria-label={t('layout.theme')}
        value={mode}
        onChange={(value) => setMode(value as ThemeMode)}
        options={themeOptions}
      />
      <Dropdown
        menu={{ items: languageItems, onClick: ({ key }) => setLocale(key as AppLocale) }}
        placement="bottomRight"
      >
        <Button icon={<GlobalOutlined />} title={t('layout.language')} />
      </Dropdown>
    </div>
  )
}

/** 品牌区装饰:请求转发拓扑线稿(客户端 → Caddy → 多站点),呼应面板的产品本质。 */
function TopologyArt({ accent, line }: { accent: string; line: string }) {
  return (
    <svg
      className="login-art"
      width="360"
      height="96"
      viewBox="0 0 360 96"
      fill="none"
      aria-hidden="true"
    >
      <path d="M28 48H120" stroke={line} strokeDasharray="4 5" />
      <circle cx="20" cy="48" r="7" stroke={line} strokeWidth="2" />
      <rect x="126" y="30" width="70" height="36" rx="8" stroke={accent} strokeWidth="2" />
      <circle cx="161" cy="48" r="3.5" fill={accent} />
      <path d="M196 48H244" stroke={line} />
      <path d="M244 48c18 0 22-30 40-30h28" stroke={line} />
      <path d="M244 48H312" stroke={line} />
      <path d="M244 48c18 0 22 30 40 30h28" stroke={line} />
      <circle cx="324" cy="18" r="6" stroke={line} strokeWidth="2" />
      <circle cx="324" cy="48" r="6" stroke={line} strokeWidth="2" />
      <circle cx="324" cy="78" r="6" stroke={line} strokeWidth="2" />
    </svg>
  )
}
