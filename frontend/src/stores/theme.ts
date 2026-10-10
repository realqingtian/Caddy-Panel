import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type ThemeMode = 'light' | 'dark' | 'system'
export type AppLocale = 'zh-CN' | 'en-US'

interface ThemeState {
  mode: ThemeMode
  locale: AppLocale
  setMode: (mode: ThemeMode) => void
  setLocale: (locale: AppLocale) => void
}

/** 外观与语言偏好;persist 到 localStorage(FR-603/604"记住选择")。 */
export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      mode: 'system',
      locale: 'zh-CN',
      setMode: (mode) => set({ mode }),
      setLocale: (locale) => set({ locale }),
    }),
    { name: 'caddy-panel-theme' },
  ),
)
