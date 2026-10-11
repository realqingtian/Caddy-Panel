import { Navigate, createBrowserRouter } from 'react-router-dom'

import { RequireAuth } from '@/components/RequireAuth'
import { AdminLayout } from '@/layouts/AdminLayout'
import ConfigEditorPage from '@/pages/ConfigEditor'
import DashboardPage from '@/pages/Dashboard'
import LoginPage from '@/pages/Login'
import LogsPage from '@/pages/Logs'
import MetricsPage from '@/pages/Metrics'
import ProfilePage from '@/pages/Profile'
import UsersPage from '@/pages/Users'
import NodesPage from '@/pages/Nodes'
import SettingsPage from '@/pages/Settings'
import TerminalPage from '@/pages/Terminal'
import SitesPage from '@/pages/Sites'
import UpstreamsPage from '@/pages/Upstreams'

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    element: <RequireAuth />,
    children: [
      {
        path: '/',
        element: <AdminLayout />,
        children: [
          { index: true, element: <DashboardPage /> },
          { path: 'sites', element: <SitesPage /> },
          { path: 'upstreams', element: <UpstreamsPage /> },
          { path: 'config', element: <ConfigEditorPage /> },
          { path: 'logs', element: <LogsPage /> },
          { path: 'metrics', element: <MetricsPage /> },
          { path: 'nodes', element: <NodesPage /> },
          { path: 'terminal', element: <TerminalPage /> },
          { path: 'settings', element: <SettingsPage /> },
          { path: 'users', element: <UsersPage /> },
          { path: '/profile', element: <ProfilePage /> },
        ],
      },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])
