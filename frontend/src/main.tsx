import { useEffect } from 'react'
import { StrictMode } from 'react'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntdApp, ConfigProvider, theme } from 'antd'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import enUS from 'antd/es/locale/en_US'
import zhCN from 'antd/es/locale/zh_CN'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router-dom'

import { useIsDarkMode } from '@/hooks/useThemeAlgorithm'
import '@/locales'
import { router } from '@/router'
import { useThemeStore, type AppLocale } from '@/stores/theme'
import '@/index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
})

const antdLocales: Record<AppLocale, typeof zhCN> = {
  'zh-CN': zhCN,
  'en-US': enUS,
}

const dayjsLocales: Record<AppLocale, string> = {
  'zh-CN': 'zh-cn',
  'en-US': 'en',
}

// antd 组件库 locale、dayjs locale 与 react-i18next 三者随语言偏好同步切换(FR-603)。
function ThemedApp() {
  const isDark = useIsDarkMode()
  const locale = useThemeStore((state) => state.locale)

  useEffect(() => {
    dayjs.locale(dayjsLocales[locale])
  }, [locale])

  return (
    <ConfigProvider
      locale={antdLocales[locale]}
      theme={{ algorithm: isDark ? theme.darkAlgorithm : theme.defaultAlgorithm }}
    >
      <AntdApp>
        <RouterProvider router={router} />
      </AntdApp>
    </ConfigProvider>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemedApp />
    </QueryClientProvider>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
