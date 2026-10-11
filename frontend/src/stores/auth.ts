import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  token: string | null
  userID: number | null
  username: string | null
  /** admin | editor | viewer;驱动前端显隐(后端仍按权限点强制校验)。 */
  role: string | null
  /** 展示名:昵称,空回退 username。 */
  nickname: string | null
  /** 登录响应标记:前端弹不可关闭的改密弹窗(服务端亦有闸门)。 */
  mustChangePassword: boolean
  setSession: (
    token: string,
    userID: number,
    username: string,
    role: string,
    nickname: string,
    mustChangePassword: boolean,
  ) => void
  setNickname: (nickname: string) => void
  clearMustChangePassword: () => void
  clear: () => void
}

/** 登录态;persist 到 localStorage 以在刷新后保持会话。 */
export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      userID: null,
      username: null,
      role: null,
      nickname: null,
      mustChangePassword: false,
      setSession: (token, userID, username, role, nickname, mustChangePassword) =>
        set({ token, userID, username, role, nickname, mustChangePassword }),
      setNickname: (nickname) => set({ nickname }),
      clearMustChangePassword: () => set({ mustChangePassword: false }),
      clear: () =>
        set({
          token: null,
          userID: null,
          username: null,
          role: null,
          nickname: null,
          mustChangePassword: false,
        }),
    }),
    { name: 'caddy-panel-auth' },
  ),
)
