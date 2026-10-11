import { useTranslation } from 'react-i18next'

/**
 * 当前界面语言对应的 BCP-47 标签,供 toLocaleString 等格式化 API 使用。
 * 时间格式必须跟随面板语言而非浏览器语言(FR-603;I4-D1)。
 */
export function useLocaleTag(): string {
  const { i18n } = useTranslation()
  return i18n.language?.startsWith('zh') ? 'zh-CN' : 'en-US'
}
