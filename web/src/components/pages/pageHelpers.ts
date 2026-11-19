import {
  ZIP_MAX_BYTES,
  ZIP_MAX_ENTRIES,
  type DropSelection,
} from '../../lib/zipSite'

export const PAGE_TABS = [
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
