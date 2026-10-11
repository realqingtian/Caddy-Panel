import { Alert, Button } from 'antd'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'

import { useAuthStore } from '@/stores/auth'
import '@xterm/xterm/css/xterm.css'

const resizeMessage = (cols: number, rows: number) => JSON.stringify({ type: 'resize', cols, rows })

type WsState = 'connecting' | 'open' | 'closed'

/** Web 终端:xterm.js ↔ WebSocket ↔ PTY(仅 admin)。终端固定深色主题,全高填充,不随页面滚动。 */
export default function TerminalPage() {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [wsState, setWsState] = useState<WsState>('connecting')
  // 重连 = 递增会话号,重建 term + WS(effect 依赖 session)
  const [session, setSession] = useState(0)

  const role = useAuthStore((s) => s.role)
  const isAdmin = role === null || role === 'admin' // 历史会话无 role 按 admin

  useEffect(() => {
    if (!isAdmin) return
    // StrictMode 双挂载下,首次实例 cleanup 触发的 ws.close() 会回调 onclose;
    // disposed 标志保证清理后的事件不再误报错误(否则已连接状态也挂红色横幅)
    let disposed = false
    // 终端始终深色(GitHub Dark),与 app 亮暗模式无关 —— 这是终端的本来面目
    const term = new Terminal({
      fontSize: 13,
      cursorBlink: true,
      theme: {
        background: '#0d1117',
        foreground: '#e6edf3',
        cursor: '#58a6ff',
        selectionBackground: '#264f78',
      },
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
    ws.onopen = () => setWsState('open')

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data))
      } else {
        term.write(String(event.data))
      }
    }
    ws.onerror = () => {
      if (disposed) return
      setError(t('terminal.connectionFailed'))
      setWsState('closed')
    }
    ws.onclose = (event) => {
      if (disposed) return
      setWsState('closed')
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
      disposed = true
      window.removeEventListener('resize', onWindowResize)
      ws.close()
      term.dispose()
    }
  }, [isAdmin, session, t])

  if (!isAdmin) {
    return <Alert type="error" showIcon message={t('terminal.adminOnly')} />
  }

  return (
    <div className="terminal-page">
      <Alert type="warning" showIcon message={t('terminal.warning')} style={{ flex: 'none' }} />
      {error && <Alert type="error" showIcon message={error} style={{ flex: 'none' }} />}
      <div className="terminal-frame">
        <div className="terminal-titlebar">
          <span className="terminal-title">{t('terminal.title')}</span>
          <span className="terminal-status" data-state={wsState}>
            {wsState === 'open'
              ? t('terminal.connected')
              : wsState === 'closed'
                ? t('terminal.closed')
                : '…'}
          </span>
          {wsState === 'closed' && (
            <Button size="small" onClick={() => setSession((s) => s + 1)}>
              {t('terminal.reconnect')}
            </Button>
          )}
        </div>
        <div ref={containerRef} className="terminal-body" />
      </div>
    </div>
  )
}
