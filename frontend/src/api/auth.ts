import axios from 'axios'

import { apiClient } from './client'
import type { ApiError } from './types'

export interface LoginResponse {
  token: string
  user_id: number
  username: string
  nickname: string
  role: 'admin' | 'editor' | 'viewer'
  must_change_password: boolean
  require_2fa?: boolean
  ticket?: string
}

export interface ProfileResponse {
  username: string
  nickname: string
  email: string
  role: string
  avatar_url: string
  totp_enabled: boolean
  must_change_password: boolean
}

export interface SessionView {
  sid: string
  device: string
  ip: string
  created_at: string
  last_seen_at: string
  current: boolean
  revoked: boolean
}

export interface UserView {
  id: number
  username: string
  nickname: string
  email: string
  role: string
  disabled: boolean
  must_change_password: boolean
  totp_enabled: boolean
  avatar_url: string
  created_at: string
  last_active_at: string
  session_count: number
}

/** 登录:成功返回 JWT;需要两步验证时返回 require_2fa + ticket(不走抛错路径)。 */
export async function login(username: string, password: string): Promise<LoginResponse> {
  const res = await apiClient.post<LoginResponse>('/auth/login', { username, password })
  return res.data
}

/** 两步验证二段登录。 */
export async function login2FA(ticket: string, code: string): Promise<LoginResponse> {
  const res = await apiClient.post<LoginResponse>('/auth/login/2fa', { ticket, code })
  return res.data
}

/** 退出登录:服务端吊销当前会话(M4 起)。 */
export async function logout(): Promise<void> {
  try {
    await apiClient.post('/auth/logout')
  } catch {
    // 服务端吊销失败不阻塞本地退出(会话随后台清扫)。
  }
}

/** 本人资料。 */
export async function getProfile(): Promise<ProfileResponse> {
  const res = await apiClient.get<ProfileResponse>('/auth/profile')
  return res.data
}

/** 维护昵称/邮箱。 */
export async function patchProfile(nickname: string, email: string): Promise<ProfileResponse> {
  const res = await apiClient.patch<ProfileResponse>('/auth/profile', { nickname, email })
  return res.data
}

/** 修改密码:成功后全部会话吊销(含当前),调用方须跳转登录页。 */
export async function changePassword(oldPassword: string, newPassword: string): Promise<void> {
  await apiClient.post('/auth/change-password', {
    old_password: oldPassword,
    new_password: newPassword,
  })
}

/** 上传自定义头像并返回刷新后的资料。 */
export async function uploadAvatar(file: File): Promise<ProfileResponse> {
  const form = new FormData()
  form.append('file', file)
  const res = await apiClient.post<ProfileResponse>('/auth/avatar', form)
  return res.data
}

/** 移除自定义头像(回退 Gravatar/首字母)。 */
export async function removeAvatar(): Promise<ProfileResponse> {
  const res = await apiClient.delete<ProfileResponse>('/auth/avatar')
  return res.data
}

/** 本人会话列表。 */
export async function listMySessions(): Promise<SessionView[]> {
  const res = await apiClient.get<SessionView[]>('/auth/sessions')
  return res.data
}

/** 下线本人指定会话。 */
export async function revokeMySession(sid: string): Promise<void> {
  await apiClient.delete(`/auth/sessions/${sid}`)
}

/** 下线本人其他设备。 */
export async function revokeOtherSessions(): Promise<void> {
  await apiClient.post('/auth/sessions/others')
}

/** 生成两步验证绑定信息。 */
export async function setup2FA(): Promise<{ secret: string; uri: string }> {
  const res = await apiClient.post<{ secret: string; uri: string }>('/auth/2fa/setup')
  return res.data
}

/** 确认启用两步验证,返回恢复码(明文仅此一次)。 */
export async function enable2FA(secret: string, code: string): Promise<string[]> {
  const res = await apiClient.post<{ codes: string[] }>('/auth/2fa/enable', { secret, code })
  return res.data.codes
}

/** 解绑两步验证(密码 + 验证码)。 */
export async function disable2FA(password: string, code: string): Promise<void> {
  await apiClient.delete('/auth/2fa', { data: { password, code } })
}

/** 重新生成恢复码(旧码作废)。 */
export async function regenerateRecoveryCodes(code: string): Promise<string[]> {
  const res = await apiClient.post<{ codes: string[] }>('/auth/2fa/recovery', { code })
  return res.data.codes
}

/** 从 axios 错误中提取后端 message(已按 Accept-Language 本地化);取不到时用兜底文案。 */
export function apiErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data as Partial<ApiError> | undefined
    if (data && typeof data.message === 'string' && data.message) {
      return data.message
    }
  }
  return fallback
}
