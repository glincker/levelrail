import { detectLogLevel, type LogLevel } from '../../lib/logLevel'
import type { LogLine } from '../../hooks/useLogStream'

export function levelOf(line: LogLine): LogLevel | 'none' {
  if (line.stream === 'stderr') return detectLogLevel(line.line) ?? 'error'
  return detectLogLevel(line.line) ?? 'none'
}
