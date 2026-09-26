import { useEffect, useRef, useState } from 'react'
import { useReducedMotion } from './useReducedMotion'

export interface TypedTextProps {
  text: string
  speedMs?: number
  onDone?: () => void
  className?: string
}

export function TypedText({
  text,
  speedMs = 18,
  onDone,
  className,
}: TypedTextProps) {
  const reduced = useReducedMotion()
  const [progress, setProgress] = useState({ text, count: 0 })
  const onDoneRef = useRef(onDone)

  useEffect(() => {
    onDoneRef.current = onDone
  })

  const count = reduced
    ? text.length
    : progress.text === text
      ? progress.count
      : 0
  const done = count >= text.length

  useEffect(() => {
    if (done) return
    let i = 0
    const id = window.setInterval(() => {
      i += 1
      setProgress({ text, count: i })
      if (i >= text.length) window.clearInterval(id)
    }, speedMs)
    return () => window.clearInterval(id)
  }, [text, speedMs, done])

  useEffect(() => {
    if (done) onDoneRef.current?.()
  }, [done, text])

  return (
    <span className={className} aria-label={text}>
      <span aria-hidden="true">{text.slice(0, count)}</span>
    </span>
  )
}
