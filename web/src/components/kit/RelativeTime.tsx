import { useEffect, useState } from 'react'
import { formatRelative } from './formatRelative'

export interface RelativeTimeProps {
  at: string | Date
  live?: boolean
}

export function RelativeTime({ at, live = false }: RelativeTimeProps) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!live) return
    const id = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(id)
  }, [live])

  const date = at instanceof Date ? at : new Date(at)
  const valid = !Number.isNaN(date.getTime())
  return (
    <time
      dateTime={valid ? date.toISOString() : undefined}
      title={valid ? date.toLocaleString() : undefined}
    >
      {formatRelative(at, now)}
    </time>
  )
}
