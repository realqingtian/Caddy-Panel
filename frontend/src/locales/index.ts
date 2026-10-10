import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import { useThemeStore } from '@/stores/theme'
import enUS from './en-US.json'
import zhCN from './zh-CN.json'

void i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: zhCN },
    'en-US': { translation: enUS },
  },
  lng: useThemeStore.getState().locale,
  fallbackLng: 'zh-CN',
  interpolation: {
    // React 已对插值结果做转义,交给 i18next 反而会双重转义。
    escapeValue: false,
  },
})

// 语言偏好持久化在 theme store;此处集中订阅,切换入口无需各自调用 changeLanguage。
useThemeStore.subscribe((state, prev) => {
  if (state.locale !== prev.locale) {
    void i18n.changeLanguage(state.locale)
  }
})

export default i18n
