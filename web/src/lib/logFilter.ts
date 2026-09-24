import type { LogLine } from '../hooks/useLogStream'
import { stripAnsiCodes } from './ansi'

export interface LogFilter {
  text: string
  stderrOnly: boolean
}

export function filterLogLines(lines: LogLine[], filter: LogFilter): LogLine[] {
  const needle = filter.text.trim().toLowerCase()
  if (needle === '' && !filter.stderrOnly) {
    return lines
  }
  return lines.filter((l) => {
    if (filter.stderrOnly && l.stream !== 'stderr') {
      return false
    }
    return (
      needle === '' || stripAnsiCodes(l.line).toLowerCase().includes(needle)
    )
  })
}

export function logLinesToText(lines: LogLine[]): string {
  return lines.map((l) => stripAnsiCodes(l.line)).join('\n')
}
