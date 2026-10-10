import axios from 'axios'

import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'

/** 业务 API 客户端:统一携带 JWT 与语言偏好;401 时清会话并跳登录页。 */
export const apiClient = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
})

apiClient.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  // 后端按 Accept-Language 返回中/英文错误消息(FR-603)。
  config.headers['Accept-Language'] = useThemeStore.getState().locale
  return config
})

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      useAuthStore.getState().clear()
      if (window.location.pathname !== '/login') {
        window.location.assign('/login')
      }
    }
    return Promise.reject(error)
  },
)
