import { useEffect, useRef, useState } from 'react'
import { useReducedMotion } from './useReducedMotion'

export interface AnimatedNumberProps {
  value: number
  format?: (n: number) => string
  durationMs?: number
}

function decimalsOf(n: number): number {
  if (!Number.isFinite(n) || Number.isInteger(n)) return 0
  const s = String(n)
  if (s.includes('e')) return 2
  return Math.min((s.split('.')[1] ?? '').length, 4)
}

const easeOut = (t: number) => 1 - Math.pow(1 - t, 3)

export function AnimatedNumber({
  value,
  format,
  durationMs = 500,
}: AnimatedNumberProps) {
  const reduced = useReducedMotion()
  const [shown, setShown] = useState(value)
  const shownRef = useRef(value)

  useEffect(() => {
    if (reduced || !Number.isFinite(value) || shownRef.current === value) {
      shownRef.current = value
      setShown(value)
      return
    }
    const from = shownRef.current
    const start = performance.now()
    let raf = 0
    const tick = () => {
      const t = Math.min((performance.now() - start) / durationMs, 1)
      const next = t >= 1 ? value : from + (value - from) * easeOut(t)
      shownRef.current = next
      setShown(next)
      if (t < 1) raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [value, durationMs, reduced])

  const decimals = decimalsOf(value)
  const text = format
    ? format(shown)
    : shown.toLocaleString(undefined, {
        minimumFractionDigits: decimals,
        maximumFractionDigits: decimals,
      })
  return (
    <span data-value={value} className="tabular-nums">
      {text}
    </span>
  )
}
