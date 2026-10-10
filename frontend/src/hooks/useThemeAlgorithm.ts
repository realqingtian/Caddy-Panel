import { useEffect, useState } from 'react'

import { useThemeStore, type ThemeMode } from '@/stores/theme'

/** 订阅系统配色偏好;不支持或未监听时按亮色兜底。 */
function usePrefersDark(): boolean {
  const [prefersDark, setPrefersDark] = useState(
    () => window.matchMedia('(prefers-color-scheme: dark)').matches,
  )

  useEffect(() => {
    const query = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (event: MediaQueryListEvent) => setPrefersDark(event.matches)
    query.addEventListener('change', onChange)
    return () => query.removeEventListener('change', onChange)
  }, [])

  return prefersDark
}

/** 三态偏好('light' | 'dark' | 'system')解析为最终是否暗色,驱动 antd algorithm。 */
export function useIsDarkMode(): boolean {
  const mode = useThemeStore((state) => state.mode)
  const prefersDark = usePrefersDark()
  return resolveDarkMode(mode, prefersDark)
}

export function resolveDarkMode(mode: ThemeMode, prefersDark: boolean): boolean {
  if (mode === 'system') return prefersDark
  return mode === 'dark'
}
