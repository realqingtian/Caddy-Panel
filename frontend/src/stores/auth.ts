import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  token: string | null
  username: string | null
  setSession: (token: string, username: string) => void
  clear: () => void
}

/** 登录态;persist 到 localStorage 以在刷新后保持会话。 */
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      username: null,
      setSession: (token, username) => set({ token, username }),
      clear: () => set({ token: null, username: null }),
    }),
    { name: 'caddy-panel-auth' },
  ),
)
