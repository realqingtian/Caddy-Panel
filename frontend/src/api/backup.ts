import { apiClient } from './client'

/** 备份文件结构(与后端 service.BackupFile 对应)。 */
export interface BackupFile {
  version: number
  exported_at: string
  sites: unknown[]
  config_versions: unknown[]
  settings: Record<string, string>
}

export interface RestoreResult {
  sites: number
  versions: number
  settings: number
}

/** 导出备份(触发 JSON 下载,FR-706)。 */
export async function exportBackup(): Promise<void> {
  const res = await apiClient.get('/backup', { responseType: 'blob' })
  const url = URL.createObjectURL(res.data as Blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `caddy-panel-backup-${new Date().toISOString().slice(0, 10)}.json`
  link.click()
  URL.revokeObjectURL(url)
}

/** 导入备份(替换站点/版本历史/设置)。 */
export async function restoreBackup(file: File): Promise<RestoreResult> {
  const content = JSON.parse(await file.text()) as BackupFile
  const res = await apiClient.post<RestoreResult>('/restore', content)
  return res.data
}
