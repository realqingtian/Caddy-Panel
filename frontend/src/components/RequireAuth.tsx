import { Navigate, Outlet } from 'react-router-dom'

import { useAuthStore } from '@/stores/auth'

/** 登录守卫:未登录访问业务页面一律重定向登录页(FR-102)。 */
export function RequireAuth() {
  const token = useAuthStore((state) => state.token)
  if (!token) {
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}
