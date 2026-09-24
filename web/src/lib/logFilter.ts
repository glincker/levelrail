import type { LogLine } from '../hooks/useLogStream'
import { stripAnsiCodes } from './ansi'
import { detectLogLevel, type LogLevel } from './logLevel'

export type LogLevelFilter = 'all' | 'errors' | 'warnings' | 'info' | 'debug'

export interface LogFilter {
  text: string
  stderrOnly: boolean
  level?: LogLevelFilter
}

const FILTER_LEVEL: Record<Exclude<LogLevelFilter, 'all'>, LogLevel> = {
  errors: 'error',
  warnings: 'warn',
  info: 'info',
  debug: 'debug',
}

export function isErrorLine(l: LogLine): boolean {
  return l.stream === 'stderr' || detectLogLevel(l.line) === 'error'
}

function matchesLevel(l: LogLine, level: LogLevelFilter): boolean {
  if (level === 'all') {
    return true
  }
  if (level === 'errors') {
    return isErrorLine(l)
  }
  return detectLogLevel(l.line) === FILTER_LEVEL[level]
}

export function filterLogLines(lines: LogLine[], filter: LogFilter): LogLine[] {
  const needle = filter.text.trim().toLowerCase()
  const level = filter.level ?? 'all'
  if (needle === '' && !filter.stderrOnly && level === 'all') {
    return lines
  }
  return lines.filter((l) => {
    if (filter.stderrOnly && l.stream !== 'stderr') {
      return false
    }
    if (!matchesLevel(l, level)) {
      return false
    }
    return (
      needle === '' || stripAnsiCodes(l.line).toLowerCase().includes(needle)
    )
  })
}

export function findFirstErrorIndex(lines: LogLine[]): number {
  return lines.findIndex(isErrorLine)
}

export function logLinesToText(lines: LogLine[]): string {
  return lines.map((l) => stripAnsiCodes(l.line)).join('\n')
}
