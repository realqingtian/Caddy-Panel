import { useEffect, useRef, useState } from 'react'

interface UsePollingOptions {
  /** 轮询间隔毫秒;paused 为 true 时不发起。 */
  intervalMs?: number
  paused?: boolean
}

/** 通用轮询 hook:立即执行一次,之后按间隔执行;组件卸载自动停止。 */
export function usePolling(
  fn: () => void | Promise<void>,
  { intervalMs = 3000, paused = false }: UsePollingOptions = {},
) {
  const [ticking, setTicking] = useState(false)
  const fnRef = useRef(fn)

  // 回调经 ref 透传,轮询 effect 不因调用方每次渲染的新引用而重启。
  useEffect(() => {
    fnRef.current = fn
  }, [fn])

  useEffect(() => {
    if (paused) return
    let cancelled = false

    const tick = async () => {
      setTicking(true)
      try {
        await fnRef.current()
      } finally {
        if (!cancelled) setTicking(false)
      }
    }
    void tick()

    const timer = window.setInterval(() => void tick(), intervalMs)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [intervalMs, paused])

  return { ticking }
}
