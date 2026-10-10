import { Navigate, createBrowserRouter } from 'react-router-dom'

import { RequireAuth } from '@/components/RequireAuth'
import { AdminLayout } from '@/layouts/AdminLayout'
import ConfigEditorPage from '@/pages/ConfigEditor'
import DashboardPage from '@/pages/Dashboard'
import LoginPage from '@/pages/Login'
import LogsPage from '@/pages/Logs'
import SettingsPage from '@/pages/Settings'
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
          { path: 'settings', element: <SettingsPage /> },
        ],
      },
    ],
  },
  { path: '*', element: <Navigate to="/" replace /> },
])
