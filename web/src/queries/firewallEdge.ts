import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'

const BASE = '/api/v1/firewall/edge'
const REFRESH_MS = 30_000

export type FwAction = 'allow' | 'deny' | 'challenge' | 'log' | 'rate_limit'
export type FwField =
  | 'path'
  | 'host'
  | 'method'
  | 'header'
  | 'query'
  | 'user_agent'
  | 'ip'
  | 'country'
export type FwOp =
  'equals' | 'starts_with' | 'contains' | 'glob' | 'in_cidr' | 'exists'
export type FwMode = 'off' | 'log' | 'challenge' | 'block'
export type FwRange = '1h' | '6h' | '24h'
export type FwOutcome =
  'allowed' | 'denied' | 'challenged' | 'logged' | 'rate_limited'

export const FW_OUTCOMES: readonly FwOutcome[] = [
  'allowed',
  'denied',
  'challenged',
  'logged',
  'rate_limited',
]

export interface FwCondition {
  field: FwField
  op: FwOp
  name?: string
  value?: string
}

// Matches internal/edgefw.Rule plus the hit counters the list adds.
export interface FwRuleInput {
  name: string
  note?: string
  scope?: string
  enabled: boolean
  conditions: FwCondition[]
  action: FwAction
  rate_count?: number
  rate_window_seconds?: number
}

export interface FwRule extends FwRuleInput {
  id: string
  position: number
  created_at?: string
  updated_at?: string
  hits: { total: number; last_hit_at?: string; sparkline: number[] }
}

export interface FwStatus {
  active: boolean
  attack_mode: { active: boolean; until?: string; challenge: boolean }
  rules_total: number
  rules_enabled: number
  bot_mode: FwMode
  ai_bot_mode: FwMode
  bans_enabled: boolean
  bans: number
  geo_active: boolean
  range: FwRange
  totals: Record<FwOutcome, number>
  reasons: { reason: string; count: number }[]
  attack_suggestion: {
    suggested: boolean
    per_minute: number
    threshold: number
    window_minutes: number
  }
  retention: {
    chart_hours: number
    top_minutes: number
    in_memory_only: boolean
    top_capacity: number
    ban_duration: string
    ban_strikes: number
    ban_window: string
  }
}

export interface FwSeriesPoint {
  time: string
  counts: Record<FwOutcome, number>
}

export interface FwTopRow {
  key: string
  count: number
  sparkline: number[]
}

export type FwTopTable = 'ip' | 'denied_ip' | 'user_agent' | 'path' | 'host'

export interface FwTraffic {
  window_minutes: number
  approximate: boolean
  tables: Record<FwTopTable, FwTopRow[]>
}

export interface FwBan {
  ip: string
  reason: string
  started: string
  ends: string
  hits: number
}

export interface FwSettings {
  bot_mode: FwMode
  ai_bot_mode: FwMode
  ai_bot_agents: string[]
  bans_enabled: boolean
  attack_until?: string
  attack_challenge: boolean
  default_ai_bot_agents: string[]
  max_ai_bot_agents: number
}

export interface FwAuditEntry {
  id: string
  actor_name: string
  actor_type: string
  action: string
  created_at: string
  detail?: Record<string, string>
}

export interface FwFieldError {
  field: string
  message: string
}

export class FirewallError extends ApiError {
  readonly code?: string
  readonly fields: FwFieldError[]

  constructor(
    status: number,
    message: string,
    code?: string,
    fields?: FwFieldError[],
  ) {
    super(status, message)
    this.name = 'FirewallError'
    this.code = code
    this.fields = fields ?? []
  }
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
  })
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
      fields?: FwFieldError[]
    } | null
    throw new FirewallError(
      res.status,
      body?.error ?? `firewall request failed: ${res.status}`,
      body?.code,
      body?.fields,
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function query(scope: string, range?: FwRange): string {
  const q = new URLSearchParams()
  if (scope) q.set('scope', scope)
  if (range) q.set('range', range)
  const s = q.toString()
  return s ? `?${s}` : ''
}

export const firewallKeys = {
  all: ['firewall'] as const,
  status: (scope: string, range: FwRange) =>
    ['firewall', 'status', scope, range] as const,
  series: (scope: string, range: FwRange) =>
    ['firewall', 'series', scope, range] as const,
  traffic: (scope: string) => ['firewall', 'traffic', scope] as const,
  rules: ['firewall', 'rules'] as const,
  bans: ['firewall', 'bans'] as const,
  settings: ['firewall', 'settings'] as const,
  audit: ['firewall', 'audit'] as const,
}

export function useFirewallStatus(scope: string, range: FwRange) {
  return useQuery(
    queryOptions({
      queryKey: firewallKeys.status(scope, range),
      queryFn: () => call<FwStatus>(`/status${query(scope, range)}`),
      refetchInterval: REFRESH_MS,
    }),
  )
}

export function useFirewallSeries(scope: string, range: FwRange) {
  return useQuery(
    queryOptions({
      queryKey: firewallKeys.series(scope, range),
      queryFn: () => call<FwSeriesPoint[]>(`/series${query(scope, range)}`),
      refetchInterval: REFRESH_MS,
    }),
  )
}

export function useFirewallTraffic(scope: string) {
  return useQuery(
    queryOptions({
      queryKey: firewallKeys.traffic(scope),
      queryFn: () => call<FwTraffic>(`/traffic${query(scope)}`),
      refetchInterval: REFRESH_MS,
    }),
  )
}

export function useFirewallRules() {
  return useQuery(
    queryOptions({
      queryKey: firewallKeys.rules,
      queryFn: () => call<FwRule[]>('/rules'),
      refetchInterval: REFRESH_MS,
    }),
  )
}

export function useFirewallBans() {
  return useQuery(
    queryOptions({
      queryKey: firewallKeys.bans,
      queryFn: () => call<FwBan[]>('/bans'),
      refetchInterval: REFRESH_MS,
    }),
  )
}

export function useFirewallSettings() {
  return useQuery(
    queryOptions({
      queryKey: firewallKeys.settings,
      queryFn: () => call<FwSettings>('/settings'),
    }),
  )
}

export function useFirewallAudit() {
  return useQuery(
    queryOptions({
      queryKey: firewallKeys.audit,
      queryFn: () => call<FwAuditEntry[]>('/audit?limit=100'),
    }),
  )
}

function useInvalidating<V, R>(fn: (v: V) => Promise<R>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => qc.invalidateQueries({ queryKey: firewallKeys.all }),
  })
}

function json(method: string, body?: unknown): RequestInit {
  return { method, body: body === undefined ? undefined : JSON.stringify(body) }
}

function forceSuffix(force?: boolean): string {
  return force ? '?force=true' : ''
}

export interface RuleWrite {
  rule: FwRuleInput
  force?: boolean
}

export function useCreateFirewallRule() {
  return useInvalidating(({ rule, force }: RuleWrite) =>
    call<FwRule>(`/rules${forceSuffix(force)}`, json('POST', rule)),
  )
}

export function useUpdateFirewallRule() {
  return useInvalidating(({ id, rule, force }: RuleWrite & { id: string }) =>
    call<FwRule>(
      `/rules/${encodeURIComponent(id)}${forceSuffix(force)}`,
      json('PUT', rule),
    ),
  )
}

export function useDeleteFirewallRule() {
  return useInvalidating((id: string) =>
    call<void>(`/rules/${encodeURIComponent(id)}`, json('DELETE')),
  )
}

export function useReorderFirewallRules() {
  return useInvalidating((ids: string[]) =>
    call<FwRule[]>('/rules/order', json('POST', { ids })),
  )
}

export interface IpEntry {
  ip: string
  scope?: string
  note?: string
  force?: boolean
}

export function useBlockFirewallIp() {
  return useInvalidating(({ force, ...body }: IpEntry) =>
    call<FwRule>(`/block${forceSuffix(force)}`, json('POST', body)),
  )
}

export function useAllowFirewallIp() {
  return useInvalidating(({ ip, scope, note }: IpEntry) =>
    call<FwRule>('/allow', json('POST', { ip, scope, note })),
  )
}

export function useLiftFirewallBan() {
  return useInvalidating((ip: string) =>
    call<void>(`/bans/${encodeURIComponent(ip)}`, json('DELETE')),
  )
}

export type FwSettingsInput = Pick<
  FwSettings,
  'bot_mode' | 'ai_bot_mode' | 'ai_bot_agents' | 'bans_enabled'
>

export function useSaveFirewallSettings() {
  return useInvalidating((s: FwSettingsInput) =>
    call<FwSettings>('/settings', json('PUT', s)),
  )
}

export function useStartFirewallAttack() {
  return useInvalidating((b: { minutes: number; challenge: boolean }) =>
    call<unknown>('/attack-mode', json('POST', b)),
  )
}

export function useStopFirewallAttack() {
  return useInvalidating(() => call<unknown>('/attack-mode', json('DELETE')))
}
