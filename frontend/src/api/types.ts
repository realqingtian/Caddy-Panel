/** 与后端约定的统一错误结构 `{code, message, detail}`。 */
export interface ApiError {
  code: string
  message: string
  detail?: unknown
}

/** 供 axios 拦截器判断的 axios 错误形态。 */
export function toApiError(payload: unknown): ApiError | null {
  if (
    payload &&
    typeof payload === 'object' &&
    'code' in payload &&
    'message' in payload &&
    typeof (payload as { code: unknown }).code === 'string' &&
    typeof (payload as { message: unknown }).message === 'string'
  ) {
    return payload as ApiError
  }
  return null
}
