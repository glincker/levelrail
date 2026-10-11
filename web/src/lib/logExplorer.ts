import type { LogQuerySpec } from '../types/logExplorer'

const PLACEHOLDER_PATTERNS: Record<string, string> = {
  '<uuid>':
    '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}',
  '<ts>': '\\S+',
  '<time>': '\\d{2}:\\d{2}:\\d{2}\\S*',
  '<date>': '\\d{4}-\\d{2}-\\d{2}',
  '<email>': '\\S+@\\S+',
  '<ip>': '[0-9a-fA-F.:]+',
  '<hex>': '\\w+',
  '<token>': '\\w+',
  '<num>': '-?\\d+(?:\\.\\d+)?',
}

const PLACEHOLDER_SPLIT =
  /(<uuid>|<ts>|<time>|<date>|<email>|<ip>|<hex>|<token>|<num>)/

function escapeRegex(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// signatureToRegex turns a signature template back into a pattern that
// matches every line of that cluster, for "alert on this error".
export function signatureToRegex(template: string): string {
  return template
    .split(PLACEHOLDER_SPLIT)
    .map((part) => PLACEHOLDER_PATTERNS[part] ?? escapeRegex(part))
    .join('')
}

const RECENT_KEY = 'levelrail.logs.recent'
const RECENT_MAX = 8

export interface RecentQuery {
  q?: string
  level?: string
  field?: string[]
}

function queryKey(r: RecentQuery): string {
  return JSON.stringify([r.q ?? '', r.level ?? '', r.field ?? []])
}

export function isEmptyQuery(r: RecentQuery): boolean {
  return !r.q && !r.level && (r.field?.length ?? 0) === 0
}

export function readRecentQueries(app: string): RecentQuery[] {
  try {
    const raw = window.localStorage.getItem(`${RECENT_KEY}.${app}`)
    const parsed: unknown = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? (parsed as RecentQuery[]) : []
  } catch {
    return []
  }
}

export function rememberQuery(app: string, r: RecentQuery): RecentQuery[] {
  if (isEmptyQuery(r)) {
    return readRecentQueries(app)
  }
  const next = [
    r,
    ...readRecentQueries(app).filter((x) => queryKey(x) !== queryKey(r)),
  ].slice(0, RECENT_MAX)
  try {
    window.localStorage.setItem(`${RECENT_KEY}.${app}`, JSON.stringify(next))
  } catch {
    return next
  }
  return next
}

export function describeQuery(r: RecentQuery): string {
  const parts: string[] = []
  if (r.q) {
    parts.push(`"${r.q}"`)
  }
  if (r.level) {
    parts.push(`>= ${r.level}`)
  }
  for (const f of r.field ?? []) {
    parts.push(f)
  }
  return parts.join(' ')
}

export function specToRecent(spec: LogQuerySpec): RecentQuery {
  return { q: spec.q, level: spec.level, field: spec.fields }
}

// Seconds in a relative range such as "15m", "1h" or "7d"; 0 when unparseable.
export function rangeSeconds(range: string | undefined): number {
  const m = /^(\d{1,4})([mhd])$/.exec(range ?? '')
  if (!m) {
    return 0
  }
  const unit = { m: 60, h: 3600, d: 86400 }[m[2] as 'm' | 'h' | 'd']
  return Number(m[1]) * unit
}
