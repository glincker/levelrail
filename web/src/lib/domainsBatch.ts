import type { TargetKind } from '../queries/parkedDomains'
import { isNoopMove, type TargetOption } from './domainLifecycle'

export type BatchAction = 'park' | 'move' | 'attach' | 'delete_parked'

export interface BatchItem {
  domain: string
  app?: string
  from?: string
  to?: string
  note?: string
  environment?: string
  target_kind?: TargetKind
  from_kind?: TargetKind
  to_kind?: TargetKind
  from_environment?: string
  to_environment?: string
}

export interface BatchItemResult {
  domain: string
  ok?: boolean
  status?: string
  error?: string
  code?: string
  requires_confirmation?: boolean
  message?: string
  restored_settings?: string[]
}

export interface BatchSummary {
  total: number
  succeeded: number
  failed: number
  requires_confirmation: number
}

export interface BatchRequest {
  action: BatchAction
  items: BatchItem[]
  confirm?: boolean
  discard_settings?: boolean
}

export interface BatchResponse {
  batch_id?: string
  action?: BatchAction
  results?: BatchItemResult[]
  summary?: BatchSummary
}

// Stays under the server's domains.batch_max_items default (50).
export const BATCH_CHUNK_SIZE = 25

export function chunk<T>(list: readonly T[], size = BATCH_CHUNK_SIZE): T[][] {
  const n = Math.max(1, Math.floor(size))
  const out: T[][] = []
  for (let i = 0; i < list.length; i += n) out.push(list.slice(i, i + n))
  return out
}

export type ResultState = 'ok' | 'needs_confirmation' | 'failed'

export function resultState(r: BatchItemResult): ResultState {
  if (r.ok === true) return 'ok'
  if (r.requires_confirmation === true) return 'needs_confirmation'
  return 'failed'
}

export function summarize(results: readonly BatchItemResult[]): BatchSummary {
  const s: BatchSummary = {
    total: results.length,
    succeeded: 0,
    failed: 0,
    requires_confirmation: 0,
  }
  for (const r of results) {
    const st = resultState(r)
    if (st === 'ok') s.succeeded++
    else if (st === 'needs_confirmation') s.requires_confirmation++
    else s.failed++
  }
  return s
}

// A later result for the same domain (a confirm resend) replaces the earlier
// one in place, so the table keeps its order.
export function mergeResults(
  prev: readonly BatchItemResult[],
  next: readonly BatchItemResult[],
): BatchItemResult[] {
  const byDomain = new Map(next.map((r) => [r.domain, r]))
  const merged = prev.map((r) => byDomain.get(r.domain) ?? r)
  const seen = new Set(prev.map((r) => r.domain))
  for (const r of next) {
    if (!seen.has(r.domain)) {
      merged.push(r)
      seen.add(r.domain)
    }
  }
  return merged
}

// One request per chunk; a failed request marks only its own items failed.
export async function runChunked(
  req: BatchRequest,
  send: (chunkReq: BatchRequest) => Promise<BatchResponse>,
  size = BATCH_CHUNK_SIZE,
): Promise<BatchItemResult[]> {
  let all: BatchItemResult[] = []
  for (const part of chunk(req.items, size)) {
    let got: BatchItemResult[]
    try {
      const res = await send({ ...req, items: part })
      got = res.results ?? []
      // Items the server did not answer for are failures, never silent.
      const answered = new Set(got.map((r) => r.domain))
      for (const it of part) {
        if (!answered.has(it.domain)) {
          got = [...got, { domain: it.domain, ok: false, error: '' }]
        }
      }
    } catch (e) {
      const error = e instanceof Error ? e.message : ''
      got = part.map((it) => ({ domain: it.domain, ok: false, error }))
    }
    all = mergeResults(all, got)
  }
  return all
}

// Items still waiting on a yes, for the Confirm these resend.
export function itemsNeedingConfirmation(
  items: readonly BatchItem[],
  results: readonly BatchItemResult[],
): BatchItem[] {
  const waiting = new Set(
    results
      .filter((r) => resultState(r) === 'needs_confirmation')
      .map((r) => r.domain),
  )
  return items.filter((i) => waiting.has(i.domain))
}

export type SelectAllState = 'none' | 'some' | 'all'

export function toggleKey(sel: ReadonlySet<string>, key: string): Set<string> {
  const next = new Set(sel)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  return next
}

export function setKeys(
  sel: ReadonlySet<string>,
  keys: readonly string[],
  on: boolean,
): Set<string> {
  const next = new Set(sel)
  for (const k of keys) {
    if (on) next.add(k)
    else next.delete(k)
  }
  return next
}

export function selectAllState(
  sel: ReadonlySet<string>,
  visibleSelectable: readonly string[],
): SelectAllState {
  const n = visibleSelectable.filter((k) => sel.has(k)).length
  if (n === 0) return 'none'
  return n === visibleSelectable.length ? 'all' : 'some'
}

// Selection outlives filtering but never points at rows that are gone.
export function resolveSelection<T>(
  sel: ReadonlySet<string>,
  rows: readonly T[],
  keyOf: (row: T) => string,
  selectable: (row: T) => boolean,
): T[] {
  return rows.filter((r) => selectable(r) && sel.has(keyOf(r)))
}

interface LiveRow {
  domain: string
  service_name: string
  environment_id?: string
}

export function parkItem(d: LiveRow): BatchItem {
  return {
    domain: d.domain,
    app: d.service_name,
    ...(d.environment_id ? { environment: d.environment_id } : {}),
  }
}

export function moveItem(
  d: LiveRow,
  to: TargetOption,
  toEnvironment: string,
): BatchItem {
  const toStatic = to.kind === 'static_site'
  return {
    domain: d.domain,
    from: d.service_name,
    to: to.name,
    ...(d.environment_id ? { from_environment: d.environment_id } : {}),
    ...(toEnvironment && !toStatic ? { to_environment: toEnvironment } : {}),
    ...(toStatic ? { to_kind: to.kind } : {}),
  }
}

export function attachItem(
  domain: string,
  to: TargetOption,
  environment: string,
): BatchItem {
  const toStatic = to.kind === 'static_site'
  return {
    domain,
    app: to.name,
    ...(environment && !toStatic ? { environment } : {}),
    ...(toStatic ? { target_kind: to.kind } : {}),
  }
}

export function isNoopBatchMove(it: BatchItem): boolean {
  return isNoopMove({
    from: it.from ?? '',
    to: it.to ?? '',
    fromEnvironment: it.from_environment,
    toEnvironment: it.to_environment,
  })
}
