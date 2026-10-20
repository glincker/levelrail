import { stripAnsiCodes } from './ansi'

export type LogLevel = 'error' | 'warn' | 'info' | 'debug'

const LEVEL_ALIASES: Record<string, LogLevel> = {
  error: 'error',
  err: 'error',
  fatal: 'error',
  panic: 'error',
  critical: 'error',
  crit: 'error',
  warn: 'warn',
  warning: 'warn',
  info: 'info',
  notice: 'info',
  debug: 'debug',
  trace: 'debug',
}

const JSON_LEVEL_KEYS = ['level', 'severity', 'lvl', 'log_level', 'loglevel']

const KV_LEVEL = /(?:^|\s)(?:level|lvl|severity)=["']?([A-Za-z]+)["']?/i
const PREFIX_LEVEL =
  /^\W{0,3}(error|err|fatal|panic|critical|crit|warning|warn|info|notice|debug|trace)\b/i
const BRACKET_LEVEL =
  /\[\s*(error|err|fatal|critical|warning|warn|info|debug|trace)\s*\]/i

function fromName(value: unknown): LogLevel | null {
  if (typeof value !== 'string') {
    return null
  }
  return LEVEL_ALIASES[value.trim().toLowerCase()] ?? null
}

export function parseJsonLine(line: string): Record<string, unknown> | null {
  const trimmed = stripAnsiCodes(line).trim()
  if (!trimmed.startsWith('{') || !trimmed.endsWith('}')) {
    return null
  }
  try {
    const parsed: unknown = JSON.parse(trimmed)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    return null
  }
  return null
}

export function detectLogLevel(rawLine: string): LogLevel | null {
  const line = stripAnsiCodes(rawLine)
  const json = parseJsonLine(line)
  if (json) {
    for (const key of JSON_LEVEL_KEYS) {
      const level = fromName(json[key])
      if (level) {
        return level
      }
    }
    return null
  }
  const kv = KV_LEVEL.exec(line)
  const kvLevel = kv ? fromName(kv[1]) : null
  if (kvLevel) {
    return kvLevel
  }
  const prefix = PREFIX_LEVEL.exec(line.trimStart())
  if (prefix) {
    return fromName(prefix[1])
  }
  const bracket = BRACKET_LEVEL.exec(line)
  return bracket ? fromName(bracket[1]) : null
}

export function prettyJsonLine(line: string): string | null {
  const json = parseJsonLine(line)
  return json ? JSON.stringify(json, null, 2) : null
}
