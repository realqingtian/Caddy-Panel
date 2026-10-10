import { Alert, Card } from 'antd'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

import { useAuthStore } from '@/stores/auth'
import { useIsDarkMode } from '@/hooks/useThemeAlgorithm'
import '@xterm/xterm/css/xterm.css'

const resizeMessage = (cols: number, rows: number) => JSON.stringify({ type: 'resize', cols, rows })

/** Web 终端:xterm.js ↔ WebSocket ↔ PTY(仅 admin,FR:M3 T3.2)。 */
export default function TerminalPage() {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const [error, setError] = useState<string | null>(null)

  const isDark = useIsDarkMode()
  const role = useAuthStore((s) => s.role)
  const isAdmin = role === null || role === 'admin' // 历史会话无 role 按 admin

  useEffect(() => {
    if (!isAdmin) return
    const term = new Terminal({
      fontSize: 13,
      cursorBlink: true,
      theme: isDark ? { background: '#1f1f1f' } : { background: '#ffffff' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(containerRef.current!)
    fit.fit()
    termRef.current = term
    term.focus()

    const token = useAuthStore.getState().token ?? ''
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(
      `${proto}://${window.location.host}/api/v1/terminal/ws?token=${encodeURIComponent(token)}`,
    )
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data))
      } else {
        term.write(String(event.data))
      }
    }
    ws.onerror = () => setError(t('terminal.connectionFailed'))
    ws.onclose = (event) => {
      if (event.code === 1008 || event.code === 1006) {
        setError(t('terminal.closed'))
      }
      term.write(`\r\n\x1b[2m${t('terminal.closedBanner')}\x1b[0m`)
    }

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })
    const sendResize = () => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(resizeMessage(term.cols, term.rows))
      }
    }
    term.onResize(sendResize)
    sendResize()

    const onWindowResize = () => fit.fit()
    window.addEventListener('resize', onWindowResize)
    return () => {
      window.removeEventListener('resize', onWindowResize)
      ws.close()
      term.dispose()
    }
  }, [isAdmin, isDark, t])

  if (!isAdmin) {
    return <Alert type="error" showIcon message={t('terminal.adminOnly')} />
  }

  return (
    <Card title={t('terminal.title')}>
      {error && <Alert type="error" showIcon message={error} style={{ marginBottom: 12 }} />}
      <Alert type="warning" showIcon message={t('terminal.warning')} style={{ marginBottom: 12 }} />
      <div ref={containerRef} style={{ height: 'calc(100vh - 320px)', minHeight: 400 }} />
    </Card>
  )
}
