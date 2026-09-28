import { detectLogLevel, type LogLevel } from '../../lib/logLevel'
import type { LogLine } from '../../hooks/useLogStream'

export const LEVEL_DOT: Record<LogLevel | 'none', string> = {
  error: 'bg-destructive',
  warn: 'bg-amber-500',
  info: 'bg-sky-500',
  debug: 'bg-muted-foreground/50',
  none: 'bg-muted-foreground/30',
}

export function levelOf(line: LogLine): LogLevel | 'none' {
  if (line.stream === 'stderr') return detectLogLevel(line.line) ?? 'error'
  return detectLogLevel(line.line) ?? 'none'
}
