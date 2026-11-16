import type { LogLevel } from '../lib/logLevel'

const LEVEL_TAG: Record<LogLevel, { label: string; className: string }> = {
  error: { label: 'ERR', className: 'text-glinr-code-log-error' },
  warn: { label: 'WRN', className: 'text-glinr-code-log-warn' },
  info: { label: 'INF', className: 'text-glinr-code-log-info' },
  debug: { label: 'DBG', className: 'text-glinr-code-log-debug' },
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
