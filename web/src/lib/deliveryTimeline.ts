// Pure helpers for the Delivery tab: classify, filter, group and parse timeline items.

import type { TimelineItem } from '../types/appDelivery'

export type DeliveryCategory =
  'deploys' | 'builds' | 'statuses' | 'previews' | 'integrations' | 'other'

export const FILTERABLE_CATEGORIES: readonly DeliveryCategory[] = [
  'deploys',
  'builds',
  'statuses',
  'previews',
  'integrations',
]

export type StatusFilter = 'all' | 'failed' | 'in_progress' | 'succeeded'

export interface DeliveryFilters {
  kinds: readonly DeliveryCategory[]
  status: StatusFilter
  search: string
}

export const EMPTY_FILTERS: DeliveryFilters = {
  kinds: [],
  status: 'all',
  search: '',
}

export function categoryOf(item: TimelineItem): DeliveryCategory {
  if (item.kind === 'commit_status') {
    return item.actor === 'preview' ? 'previews' : 'statuses'
  }
  if (item.kind === 'connection_bound') return 'integrations'
  if (item.kind.includes('preview')) return 'previews'
  if (item.kind.includes('build')) return 'builds'
  if (item.kind === 'deploy' || item.kind === 'rollback') return 'deploys'
  return 'other'
}

export function filterItems(
  items: readonly TimelineItem[],
  f: DeliveryFilters,
): TimelineItem[] {
  const q = f.search.trim().toLowerCase()
  return items.filter((item) => {
    if (f.kinds.length > 0 && !f.kinds.includes(categoryOf(item))) return false
    if (f.status !== 'all' && item.status !== f.status) return false
    if (q === '') return true
    const hay = `${item.title} ${item.detail ?? ''} ${item.actor} ${item.kind}`
    return hay.toLowerCase().includes(q)
  })
}

export function isFiltered(f: DeliveryFilters): boolean {
  return f.kinds.length > 0 || f.status !== 'all' || f.search.trim() !== ''
}

export interface DayGroup {
  day: string
  items: TimelineItem[]
}

// Local calendar day, so "Today" matches what the viewer's clock says.
export function dayKey(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return 'unknown'
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

// Keeps input order (newest first), starting a new group whenever the day changes.
export function groupByDay(items: readonly TimelineItem[]): DayGroup[] {
  const groups: DayGroup[] = []
  for (const item of items) {
    const key = dayKey(item.at)
    const last = groups[groups.length - 1]
    if (last && last.day === key) last.items.push(item)
    else groups.push({ day: key, items: [item] })
  }
  return groups
}

export interface CommitStatusInfo {
  sha: string
  provider: string
  description: string
  error: string
}

const SHA_RE = /\bon ([0-9a-f]{7,40})\b/i

// Title is "<context> <state> on <sha>"; detail is "provider; description; error".
export function parseCommitStatus(item: TimelineItem): CommitStatusInfo {
  const sha = SHA_RE.exec(item.title)?.[1]?.slice(0, 7) ?? ''
  const parts = (item.detail ?? '').split(';').map((p) => p.trim())
  const provider = parts[0] ?? ''
  let description = ''
  let error = ''
  if (parts.length >= 3) {
    error = parts[parts.length - 1] ?? ''
    description = parts.slice(1, -1).join('; ')
  } else if (parts.length === 2) {
    description = parts[1] ?? ''
  }
  return { sha, provider, description, error }
}

export function latestOfKind(
  items: readonly TimelineItem[],
  kind: string,
): TimelineItem | undefined {
  let best: TimelineItem | undefined
  for (const item of items) {
    if (item.kind !== kind) continue
    if (!best || Date.parse(item.at) > Date.parse(best.at)) best = item
  }
  return best
}

export function postStatusFailing(items: readonly TimelineItem[]): boolean {
  return latestOfKind(items, 'commit_status')?.status === 'failed'
}
