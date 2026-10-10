import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  token: string | null
  username: string | null
  /** admin | viewer;决定终端入口显隐(后端仍强制校验)。 */
  role: string | null
  setSession: (token: string, username: string, role: string) => void
  clear: () => void
}

/** 登录态;persist 到 localStorage 以在刷新后保持会话。 */
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      username: null,
      role: null,
      setSession: (token, username, role) => set({ token, username, role }),
      clear: () => set({ token: null, username: null, role: null }),
    }),
    { name: 'caddy-panel-auth' },
  ),
)
