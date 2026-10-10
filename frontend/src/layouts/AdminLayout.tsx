import { useState, type ReactNode } from 'react'

import {
  ControlOutlined,
  DashboardOutlined,
  DesktopOutlined,
  FileTextOutlined,
  GlobalOutlined,
  LogoutOutlined,
  MoonOutlined,
  SettingOutlined,
  SunOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { App, Button, Dropdown, Layout, Menu, Segmented } from 'antd'
import { useTranslation } from 'react-i18next'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'

import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type AppLocale, type ThemeMode } from '@/stores/theme'

const { Sider, Header, Content } = Layout

const themeOptions: { value: ThemeMode; icon: ReactNode }[] = [
  { value: 'light', icon: <SunOutlined /> },
  { value: 'dark', icon: <MoonOutlined /> },
  { value: 'system', icon: <DesktopOutlined /> },
]

/** 管理后台布局:Sider 导航 + Header(语言/主题/用户)。 */
export function AdminLayout() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const [collapsed, setCollapsed] = useState(false)

  const menuItems = [
    { key: '/', icon: <DashboardOutlined />, label: t('menu.dashboard') },
    { key: '/sites', icon: <GlobalOutlined />, label: t('menu.sites') },
    { key: '/config', icon: <ControlOutlined />, label: t('menu.config') },
    { key: '/logs', icon: <FileTextOutlined />, label: t('menu.logs') },
    { key: '/settings', icon: <SettingOutlined />, label: t('menu.settings') },
  ]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider collapsible collapsed={collapsed} onCollapse={setCollapsed}>
        <div className="layout-logo">{t('app.title')}</div>
        <Menu
          theme="dark"
          mode="inline"
          items={menuItems}
          selectedKeys={[location.pathname]}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header className="layout-header">
          <HeaderActions />
        </Header>
        <Content style={{ margin: 16 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}

/** Header 右侧操作区:主题三态切换、语言切换、用户菜单(FR-603/604)。 */
function HeaderActions() {
  const { t } = useTranslation()
  const { message } = App.useApp()
  const navigate = useNavigate()

  const mode = useThemeStore((state) => state.mode)
  const setMode = useThemeStore((state) => state.setMode)
  const setLocale = useThemeStore((state) => state.setLocale)
  const username = useAuthStore((state) => state.username)
  const clear = useAuthStore((state) => state.clear)

  const onLogout = () => {
    clear()
    void message.success(t('layout.logoutSuccess'))
    navigate('/login', { replace: true })
  }

  // 语言选项显示语言自称(两种语言下文案一致,统一走 i18n 文件维护)。
  const languageItems = [
    { key: 'zh-CN', label: t('layout.langZhCN') },
    { key: 'en-US', label: t('layout.langEnUS') },
  ]

  return (
    <div className="layout-header-actions">
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
      <Dropdown
        menu={{
          items: [{ key: 'logout', icon: <LogoutOutlined />, label: t('layout.logout') }],
          onClick: ({ key }) => {
            if (key === 'logout') onLogout()
          },
        }}
        placement="bottomRight"
      >
        <Button icon={<UserOutlined />}>{username ?? t('layout.user')}</Button>
      </Dropdown>
    </div>
  )
}
