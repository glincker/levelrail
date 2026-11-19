import {
  ZIP_MAX_BYTES,
  ZIP_MAX_ENTRIES,
  type DropSelection,
} from '../../lib/zipSite'

export const PAGE_TABS = [
  'overview',
  'deployments',
  'domains',
  'routes',
  'settings',
] as const
export type PageTab = (typeof PAGE_TABS)[number]

export function suggestSiteName(taken: readonly string[]): string {
  if (!taken.includes('my-site')) return 'my-site'
  for (let i = 2; i < 1000; i++) {
    if (!taken.includes(`my-site-${i}`)) return `my-site-${i}`
  }
  return 'my-site'
}

export interface SelectionSummaryData {
  files: number | null
  bytes: number
  hasIndex: boolean | null
  redirects: boolean
  headers: boolean
  overLimit: boolean
}

/** What the drop holds, as far as the browser can tell; a zip is opaque so only its size is known. */
export function summarizeSelection(s: DropSelection): SelectionSummaryData {
  if (s.kind === 'zip') {
    return {
      files: null,
      bytes: s.bytes,
      hasIndex: null,
      redirects: false,
      headers: false,
      overLimit: s.bytes > ZIP_MAX_BYTES,
    }
  }
  const has = (name: string) => s.files.some((f) => f.path === name)
  return {
    files: s.files.length,
    bytes: s.bytes,
    hasIndex: s.hasIndex,
    redirects: has('_redirects'),
    headers: has('_headers'),
    overLimit: s.bytes > ZIP_MAX_BYTES || s.files.length > ZIP_MAX_ENTRIES,
  }
}

/** Cells in the tiny status split bar; widths are whole cells so no inline style is needed. */
export const STATUS_CELLS = 24

export const STATUS_CLASSES = {
  '2xx': 'bg-emerald-500',
  '3xx': 'bg-sky-500',
  '4xx': 'bg-amber-500',
  '5xx': 'bg-red-500',
} as const

/** Largest-remainder split of `cells` across the counts; an empty total yields all zeros. */
export function splitCells(counts: readonly number[], cells: number): number[] {
  const total = counts.reduce((a, b) => a + b, 0)
  if (total <= 0) return counts.map(() => 0)
  const exact = counts.map((c) => (c / total) * cells)
  const out = exact.map(Math.floor)
  let left = cells - out.reduce((a, b) => a + b, 0)
  const order = exact
    .map((e, i) => ({ i, r: e - Math.floor(e) }))
    .sort((a, b) => b.r - a.r)
  for (const { i } of order) {
    if (left <= 0) break
    out[i] = (out[i] ?? 0) + 1
    left--
  }
  return out
}
