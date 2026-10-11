import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'

import {
  ApiOutlined,
  ClusterOutlined,
  CodeOutlined,
  ControlOutlined,
  CheckOutlined,
  DashboardOutlined,
  DesktopOutlined,
  FileTextOutlined,
  GlobalOutlined,
  LineChartOutlined,
  LogoutOutlined,
  MoonOutlined,
  SettingOutlined,
  SunOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { App, Dropdown, Layout, Menu, Modal, Space, Typography, theme } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'

import { BrandMark } from '@/components/BrandMark'
import { logout as apiLogout, getProfile } from '@/api/auth'
import { getSettings } from '@/api/config'
import { setGravatarMirror } from '@/lib/gravatar'
import { UserAvatar } from '@/components/UserAvatar'
import { usePermission } from '@/hooks/usePermission'
import { PasswordForm } from '@/pages/Profile'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type AppLocale, type ThemeMode } from '@/stores/theme'

const { Sider, Header, Content } = Layout

/** 管理后台布局:Sider 导航(权限显隐)+ Header(页面标题/主题/语言/用户)+ 强制改密闸门。 */
export function AdminLayout() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const { token } = theme.useToken()
  const { can } = usePermission()
  const [collapsed, setCollapsed] = useState(false)

  const mustChange = useAuthStore((state) => state.mustChangePassword)
  const clearSession = useAuthStore((s) => s.clear)

  // 菜单按权限点显隐(§3.4);后端仍按权限点强制校验。
  const menuItems = [
    { key: '/', icon: <DashboardOutlined />, label: t('menu.dashboard') },
    { key: '/sites', icon: <GlobalOutlined />, label: t('menu.sites') },
    { key: '/upstreams', icon: <ApiOutlined />, label: t('menu.upstreams') },
    { key: '/nodes', icon: <ClusterOutlined />, label: t('menu.nodes') },
    ...(can('config:read')
      ? [{ key: '/config', icon: <ControlOutlined />, label: t('menu.config') }]
      : []),
    ...(can('logs:read')
      ? [{ key: '/logs', icon: <FileTextOutlined />, label: t('menu.logs') }]
      : []),
    ...(can('metrics:read')
      ? [{ key: '/metrics', icon: <LineChartOutlined />, label: t('menu.metrics') }]
      : []),
    ...(can('terminal:use')
      ? [{ key: '/terminal', icon: <CodeOutlined />, label: t('menu.terminal') }]
      : []),
    ...(can('settings:read')
      ? [{ key: '/settings', icon: <SettingOutlined />, label: t('menu.settings') }]
      : []),
    ...(can('users:manage')
      ? [{ key: '/users', icon: <UserOutlined />, label: t('menu.users') }]
      : []),
  ]
  // /profile 不在侧栏菜单中,单独回退标题(I4-D4)
  const pageTitle =
    menuItems.find((item) => item.key === location.pathname)?.label ??
    (location.pathname === '/profile' ? t('profile.title') : undefined)

  // Gravatar 镜像:登录后拉取一次设置(设置页修改后刷新页面生效)。
  useEffect(() => {
    void getSettings()
      .then((settings) => setGravatarMirror(settings.gravatar_mirror_url ?? ''))
      .catch(() => undefined)
  }, [])

  // 强制改密后当前 token 已吊销:回登录页。
  const onForcedDone = () => {
    clearSession()
    navigate('/login', { replace: true })
  }

  return (
    // 应用壳:外层固定视口不滚动,Sider/Header 常驻,仅内容区独立滚动
    <Layout style={{ height: '100vh', overflow: 'hidden' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        style={
          {
            // 背景与内容区同源(token),交由亮/暗 algorithm 自适应,避免 dark 主题深藏青与中性灰内容区打架
            background: token.colorBgContainer,
            borderRight: `1px solid ${token.colorSplit}`,
            height: '100vh',
            overflowY: 'auto',
            '--sider-trigger-bg': token.colorBgContainer,
            '--sider-trigger-fg': token.colorTextSecondary,
          } as CSSProperties
        }
      >
        <div className="layout-logo" style={{ color: token.colorText }}>
          <BrandMark color={token.colorPrimary} />
          <span className={collapsed ? 'layout-logo-text collapsed' : 'layout-logo-text'}>
            {t('app.title')}
          </span>
        </div>
        <Menu
          mode="inline"
          items={menuItems}
          selectedKeys={[location.pathname]}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header
          className="layout-header"
          style={{
            background: token.colorBgContainer,
            borderBottom: `1px solid ${token.colorSplit}`,
          }}
        >
          <Typography.Title level={4} style={{ margin: 0 }}>
            {pageTitle}
          </Typography.Title>
          <HeaderActions />
        </Header>
        {/* key=pathname:路由切换时重新触发淡入动画(见 index.css .page-fade) */}
        <Content style={{ padding: 16, overflowY: 'auto', minHeight: 0 }}>
          <div key={location.pathname} className="page-fade">
            <Outlet />
          </div>
        </Content>
      </Layout>

      {/* 强制改密:服务端闸门之外的引导弹窗(不可关闭);完成后当前 token 已吊销,回登录页 */}
      <Modal
        open={mustChange}
        title={t('profile.forced.title')}
        closable={false}
        footer={null}
        maskClosable={false}
      >
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Typography.Text type="secondary">{t('profile.forced.desc')}</Typography.Text>
          <PasswordForm requireOld onDone={onForcedDone} />
        </Space>
      </Modal>
    </Layout>
  )
}

/** Header 右侧操作区:头像聚合菜单 —— 用户信息/外观/语言/个人设置/退出(I5,交互重组)。
 *  主题与语言从 Header 常驻按钮收进下拉,降低视觉噪音;选项结构对齐主流面板习惯。 */
function HeaderActions() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const navigate = useNavigate()

  const mode = useThemeStore((state) => state.mode)
  const setMode = useThemeStore((state) => state.setMode)
  const locale = useThemeStore((state) => state.locale)
  const setLocale = useThemeStore((state) => state.setLocale)
  const username = useAuthStore((state) => state.username)
  const nickname = useAuthStore((state) => state.nickname)
  const role = useAuthStore((state) => state.role)
  const clear = useAuthStore((state) => state.clear)
  // 头像/邮箱来自 profile(react-query 缓存与 Profile 页共享,更新后自动同步)
  const profileQuery = useQuery({ queryKey: ['profile'], queryFn: getProfile })
  const email = profileQuery.data?.email
  const avatarURL = profileQuery.data?.avatar_url

  const onLogout = async () => {
    // M4:退出为服务端动作(吊销当前会话),失败不阻塞本地退出。
    await apiLogout()
    clear()
    void message.success(t('layout.logoutSuccess'))
    navigate('/login', { replace: true })
  }

  const roleText = role ? t(`users.role.${role}`) : ''
  const modeText = {
    light: t('layout.themeLight'),
    dark: t('layout.themeDark'),
    system: t('layout.themeSystem'),
  }[mode]
  const localeText = locale === 'zh-CN' ? t('layout.langZhCN') : t('layout.langEnUS')

  const themeChoice = (value: ThemeMode, icon: ReactNode) => ({
    key: value,
    icon: mode === value ? <CheckOutlined /> : icon,
    label: t(`layout.theme${value[0].toUpperCase()}${value.slice(1)}`),
    onClick: () => setMode(value),
  })

  return (
    <div className="layout-header-actions">
      <Dropdown
        menu={{
          items: [
            {
              key: 'who',
              type: 'group',
              label: (
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '4px 0' }}>
                  <UserAvatar
                    username={username ?? ''}
                    nickname={nickname ?? undefined}
                    email={email ?? undefined}
                    avatarURL={avatarURL ?? undefined}
                    size={34}
                  />
                  <div>
                    <div>{nickname || username || t('layout.user')}</div>
                    {roleText && (
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        {roleText}
                      </Typography.Text>
                    )}
                  </div>
                </div>
              ),
            },
            { type: 'divider' },
            {
              key: 'theme',
              icon: <SunOutlined />,
              label: (
                <span>
                  {t('layout.theme')}
                  <Typography.Text type="secondary" style={{ fontSize: 12, marginLeft: 16 }}>
                    {modeText}
                  </Typography.Text>
                </span>
              ),
              children: [
                themeChoice('light', <SunOutlined />),
                themeChoice('dark', <MoonOutlined />),
                themeChoice('system', <DesktopOutlined />),
              ],
            },
            {
              key: 'language',
              icon: <GlobalOutlined />,
              label: (
                <span>
                  {t('layout.language')}
                  <Typography.Text type="secondary" style={{ fontSize: 12, marginLeft: 16 }}>
                    {localeText}
                  </Typography.Text>
                </span>
              ),
              children: [
                {
                  key: 'zh-CN',
                  icon: locale === 'zh-CN' ? <CheckOutlined /> : undefined,
                  label: t('layout.langZhCN'),
                  onClick: () => setLocale('zh-CN' as AppLocale),
                },
                {
                  key: 'en-US',
                  icon: locale === 'en-US' ? <CheckOutlined /> : undefined,
                  label: t('layout.langEnUS'),
                  onClick: () => setLocale('en-US' as AppLocale),
                },
              ],
            },
            { type: 'divider' },
            { key: 'profile', icon: <UserOutlined />, label: t('profile.title') },
            { key: 'logout', icon: <LogoutOutlined />, label: t('layout.logout') },
          ],
          onClick: ({ key }) => {
            if (key === 'logout') void onLogout()
            if (key === 'profile') navigate('/profile')
          },
        }}
        placement="bottomRight"
        trigger={['click']}
      >
        {/* 官方模式:无边框圆形头像直接作触发器,hover 淡出背景,不再用大按钮包裹(I5 反馈) */}
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 40,
            height: 40,
            borderRadius: '50%',
            cursor: 'pointer',
            transition: 'background 0.2s',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(128,128,128,0.18)')}
          onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
        >
          <UserAvatar
            username={username ?? ''}
            nickname={nickname ?? undefined}
            email={email ?? undefined}
            avatarURL={avatarURL ?? undefined}
            size={32}
          />
        </span>
      </Dropdown>
    </div>
  )
}
