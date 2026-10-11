import Editor, { type OnMount } from '@monaco-editor/react'
import { useTranslation } from 'react-i18next'

import { useIsDarkMode } from '@/hooks/useThemeAlgorithm'

/** Caddyfile 语言的 monarch tokenizer 定义(轻量:注释/字符串/指令/地址/括号)。 */
const caddyfileLanguage = {
  defaultToken: '',
  tokenPostfix: '.caddyfile',
  keywords: [
    'admin',
    'log',
    'metrics',
    'servers',
    'reverse_proxy',
    'file_server',
    'root',
    'respond',
    'redir',
    'header',
    'encode',
    'php_fastcgi',
    'handle',
    'handle_path',
    'route',
    'rewrite',
    'try_files',
    'tls',
    'basicauth',
    'error',
    'abort',
    'templates',
    'vars',
    'map',
    'import',
    'flush_interval',
    'lb_policy',
    'output',
    'level',
    'protocols',
  ],
  symbols: /[{}]/,
  tokenizer: {
    root: [
      [/#.*$/, 'comment'],
      [/".*?"/, 'string'],
      [/[a-zA-Z0-9_.:-]+/, { cases: { '@keywords': 'keyword', '@default': 'identifier' } }],
      [/[{}]/, '@brackets'],
      [/\d+/, 'number'],
    ],
  },
}

interface CaddyfileEditorProps {
  /** 受控值;作为 antd Form.Item 子元素时由表单注入,可省略。 */
  value?: string
  onChange?: (value: string) => void
  readOnly?: boolean
  height?: string
}

/** Monaco 封装:注册 Caddyfile 高亮并跟随面板主题(FR-604)。 */
export function CaddyfileEditor({
  value,
  onChange,
  readOnly = false,
  height = '480px',
}: CaddyfileEditorProps) {
  const { t } = useTranslation()
  const isDark = useIsDarkMode()

  const onMount: OnMount = (_editor, monaco) => {
    if (!monaco.languages.getLanguages().some((l: { id: string }) => l.id === 'caddyfile')) {
      monaco.languages.register({ id: 'caddyfile' })
      monaco.languages.setMonarchTokensProvider('caddyfile', caddyfileLanguage)
    }
  }

  return (
    <Editor
      height={height}
      defaultLanguage="caddyfile"
      language="caddyfile"
      value={value ?? ''}
      onChange={(v) => onChange?.(v ?? '')}
      onMount={onMount}
      theme={isDark ? 'vs-dark' : 'vs'}
      options={{ readOnly, minimap: { enabled: false }, fontSize: 13, automaticLayout: true }}
      loading={t('common.loading')}
    />
  )
}
