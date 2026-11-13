import type { LogLevel } from '../lib/logLevel'

const LEVEL_TAG: Record<LogLevel, { label: string; className: string }> = {
  error: { label: 'ERR', className: 'text-red-400/80' },
  warn: { label: 'WRN', className: 'text-amber-400/80' },
  info: { label: 'INF', className: 'text-sky-400/70' },
  debug: { label: 'DBG', className: 'text-neutral-400' },
}

/** Level as visible text (not color alone) with the full name for screen readers. */
export function LogLevelTag({
  level,
  className = '',
}: {
  level: LogLevel | null | 'none'
  className?: string
}) {
  const tag = level && level !== 'none' ? LEVEL_TAG[level] : null
  return (
    <span
      className={`w-7 shrink-0 text-xs font-semibold select-none ${tag?.className ?? ''} ${className}`}
    >
      <span aria-hidden="true">{tag?.label ?? ''}</span>
      {tag && level ? <span className="sr-only">{level}</span> : null}
    </span>
  )
}
