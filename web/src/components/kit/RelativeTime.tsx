import { useEffect, useState } from 'react'
import { formatRelative } from './formatRelative'
import { formatAbsolute } from '../../lib/dateFormat'
import { setVisibleInterval } from '../../lib/pageVisibility'

export interface RelativeTimeProps {
  at: string | Date
  live?: boolean
}

export function RelativeTime({ at, live = false }: RelativeTimeProps) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!live) return
    const stop = setVisibleInterval(() => setNow(Date.now()), 30_000)
    return () => stop()
  }, [live])

  const date = at instanceof Date ? at : new Date(at)
  const valid = !Number.isNaN(date.getTime())
  return (
    <time
      dateTime={valid ? date.toISOString() : undefined}
      title={valid ? formatAbsolute(date) : undefined}
    >
      {formatRelative(at, now)}
    </time>
  )
}
