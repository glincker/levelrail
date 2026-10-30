import { useCallback, useEffect, useRef, useState } from 'react'
import { RECHECK_DELAYS_MS } from './dnsSetup'

/**
 * Runs `run` at increasing delays while `active` and not `done`, then stops.
 * Also refreshes on window focus. Timers are cleared on unmount.
 */
export function useBoundedRecheck({
  active,
  done,
  run,
  delays = RECHECK_DELAYS_MS,
}: {
  active: boolean
  done: boolean
  run: () => Promise<unknown>
  delays?: number[]
}) {
  const [attempt, setAttempt] = useState(0)
  const runRef = useRef(run)
  useEffect(() => {
    runRef.current = run
  })
  const exhausted = attempt >= delays.length
  useEffect(() => {
    if (!active || done || exhausted) return
    const id = window.setTimeout(() => {
      void Promise.resolve(runRef.current()).finally(() =>
        setAttempt((a) => a + 1),
      )
    }, delays[attempt])
    return () => window.clearTimeout(id)
  }, [active, done, exhausted, attempt, delays])
  useEffect(() => {
    if (!active) return
    const onFocus = () => void runRef.current()
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [active])
  const restart = useCallback(() => setAttempt(0), [])
  return { attempt, exhausted: exhausted && !done, restart }
}
