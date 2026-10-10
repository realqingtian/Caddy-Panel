import axios from 'axios'

import { apiClient } from './client'
import type { ApiError } from './types'

export interface LoginResponse {
  token: string
  username: string
}

/** 登录:成功返回 JWT;失败抛出 axios 错误(响应体为统一 {code,message,detail})。 */
export async function login(username: string, password: string): Promise<LoginResponse> {
  const res = await apiClient.post<LoginResponse>('/auth/login', { username, password })
  return res.data
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
