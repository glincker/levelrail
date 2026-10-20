import type {
  AiAssistantConfig,
  AiSelection,
} from '../../queries/aiAssistantSettings'
import type {
  AiProvider,
  AiProviderStatus,
  AiUsageDay,
} from '../../queries/aiProviders'

export type PillState =
  | 'not_connected'
  | 'connected'
  | 'invalid_key'
  | 'unreachable'
  | 'quota_exhausted'

export type HeaderState =
  | { kind: 'not_configured' }
  | { kind: 'ready'; active: AiSelection }
  | { kind: 'attention'; reason: 'provider' | 'no_active'; provider?: string }

const BAD_STATUS: ReadonlySet<AiProviderStatus> = new Set([
  'invalid_key',
  'unreachable',
  'quota_exhausted',
])

export function pillState(p: AiProvider): PillState {
  if (!p.has_key) return 'not_connected'
  if (p.status === 'invalid_key') return 'invalid_key'
  if (p.status === 'unreachable') return 'unreachable'
  if (p.status === 'quota_exhausted') return 'quota_exhausted'
  return 'connected'
}

export function isConnected(p: AiProvider): boolean {
  return p.has_key
}

export function isSelfHosted(p: AiProvider): boolean {
  return p.kind === 'openai_compat' && p.base_url_editable
}

export interface ProviderGroups {
  hosted: AiProvider[]
  selfHosted: AiProvider[]
}

// Connected providers first, then input order (a stable sort keeps the API's order).
export function groupProviders(providers: AiProvider[]): ProviderGroups {
  const ordered = [...providers].sort(
    (a, b) => Number(isConnected(b)) - Number(isConnected(a)),
  )
  return {
    hosted: ordered.filter((p) => !isSelfHosted(p)),
    selfHosted: ordered.filter(isSelfHosted),
  }
}

export function deriveHeader(
  providers: AiProvider[],
  config: AiAssistantConfig | undefined,
): HeaderState {
  const connected = providers.filter(isConnected)
  const active = config?.active
  if (connected.length === 0 && !active) return { kind: 'not_configured' }
  if (!active) return { kind: 'attention', reason: 'no_active' }
  const owner = providers.find((p) => p.id === active.provider)
  if (!owner || !owner.has_key || BAD_STATUS.has(owner.status)) {
    return { kind: 'attention', reason: 'provider', provider: active.provider }
  }
  return { kind: 'ready', active }
}

export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  if (from === to || from < 0 || from >= list.length) return [...list]
  const clamped = Math.max(0, Math.min(to, list.length - 1))
  const next = [...list]
  const item = list[from]
  if (item === undefined) return [...list]
  next.splice(from, 1)
  next.splice(clamped, 0, item)
  return next
}

export function sameSelection(a: AiSelection, b: AiSelection): boolean {
  return a.provider === b.provider && a.model === b.model
}

export function addFallback(
  fallback: readonly AiSelection[],
  active: AiSelection | undefined,
  next: AiSelection,
): AiSelection[] {
  if (active && sameSelection(active, next)) return [...fallback]
  if (fallback.some((f) => sameSelection(f, next))) return [...fallback]
  return [...fallback, next]
}

export interface Draft {
  apiKey: string
  baseUrl: string
  model: string
}

export interface DraftCheck {
  valid: boolean
  dirty: boolean
  baseUrlInvalid: boolean
  needsKey: boolean
}

function validBaseUrl(raw: string): boolean {
  if (!raw.trim()) return true
  try {
    const u = new URL(raw.trim())
    return u.protocol === 'http:' || u.protocol === 'https:'
  } catch {
    return false
  }
}

export function checkDraft(p: AiProvider, d: Draft): DraftCheck {
  const key = d.apiKey.trim()
  const savedBase = p.base_url ?? ''
  const baseChanged = p.base_url_editable && d.baseUrl.trim() !== savedBase
  const modelChanged = d.model.trim() !== p.default_model
  const needsKey = !isSelfHosted(p) && !p.has_key
  const baseUrlInvalid = p.base_url_editable && !validBaseUrl(d.baseUrl)
  const keyOk = !needsKey || key.length > 0
  const dirty =
    key.length > 0 ||
    baseChanged ||
    modelChanged ||
    (isSelfHosted(p) && !p.has_key)
  return {
    valid: keyOk && !baseUrlInvalid && dirty,
    dirty,
    baseUrlInvalid,
    needsKey,
  }
}

export interface DayPoint {
  date: string
  tokens: number
}

export function tokensPerDay(days: AiUsageDay[], span = 30, end = new Date()) {
  const totals = new Map<string, number>()
  for (const row of days) {
    const sum = (row.prompt_tokens || 0) + (row.completion_tokens || 0)
    totals.set(row.date, (totals.get(row.date) ?? 0) + sum)
  }
  const out: DayPoint[] = []
  for (let i = span - 1; i >= 0; i--) {
    const d = new Date(
      Date.UTC(end.getUTCFullYear(), end.getUTCMonth(), end.getUTCDate() - i),
    )
    const key = d.toISOString().slice(0, 10)
    out.push({ date: key, tokens: totals.get(key) ?? 0 })
  }
  return out
}

export interface ModelUsage {
  provider: string
  model: string
  tokens: number
  requests: number
  errors: number
}

export function usageByModel(days: AiUsageDay[]): ModelUsage[] {
  const map = new Map<string, ModelUsage>()
  for (const row of days) {
    const key = `${row.provider}\u0000${row.model}`
    const cur = map.get(key) ?? {
      provider: row.provider,
      model: row.model,
      tokens: 0,
      requests: 0,
      errors: 0,
    }
    cur.tokens += (row.prompt_tokens || 0) + (row.completion_tokens || 0)
    cur.requests += row.requests || 0
    cur.errors += row.errors || 0
    map.set(key, cur)
  }
  return [...map.values()].sort((a, b) => b.tokens - a.tokens)
}

export function formatCount(n: number): string {
  if (n >= 1_000_000) return `${+(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${+(n / 1_000).toFixed(1)}K`
  return String(n)
}
