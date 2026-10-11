import { apiClient } from './client'
import type { SessionView, UserView } from './auth'

/** 用户列表(admin)。 */
export async function listUsers(): Promise<UserView[]> {
  const res = await apiClient.get<UserView[]>('/users')
  return res.data
}

/** 创建用户请求体。 */
export interface CreateUserRequest {
  username: string
  password: string
  nickname?: string
  email?: string
  role: string
  must_change_password?: boolean
}

/** 创建用户(admin)。 */
export async function createUser(req: CreateUserRequest): Promise<UserView> {
  const res = await apiClient.post<UserView>('/users', req)
  return res.data
}

/** 修改用户请求体(字段按需)。 */
export interface UpdateUserRequest {
  nickname?: string
  email?: string
  role?: string
  disabled?: boolean
  new_password?: string
}

/** 修改用户(admin);角色/禁用/重置密码会吊销目标全部会话。 */
export async function updateUser(id: number, req: UpdateUserRequest): Promise<UserView> {
  const res = await apiClient.patch<UserView>(`/users/${id}`, req)
  return res.data
}

/** 删除用户(软删除)。 */
export async function deleteUser(id: number): Promise<void> {
  await apiClient.delete(`/users/${id}`)
}

/** 查看指定用户会话(admin)。 */
export async function listUserSessions(id: number): Promise<SessionView[]> {
  const res = await apiClient.get<SessionView[]>(`/users/${id}/sessions`)
  return res.data
}

/** 强制下线指定用户的会话(admin)。 */
export async function revokeUserSession(id: number, sid: string): Promise<void> {
  await apiClient.delete(`/users/${id}/sessions/${sid}`)
}
