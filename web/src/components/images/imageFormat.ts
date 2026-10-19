import { formatBytes } from '../../lib/format'
import type { ImageEntry } from '../../queries/imageInventory'

export const SOURCE_KEYS = [
  'webhook',
  'manual',
  'image',
  'compose',
  'promote',
  'auto_rollback',
  'clone',
  'schedule',
] as const

export type SourceKey = (typeof SOURCE_KEYS)[number] | 'other'

export function sourceKey(source: string | undefined): SourceKey {
  return SOURCE_KEYS.find((k) => k === source) ?? 'other'
}

export function sizeLabel(bytes: number): string {
  return bytes > 0 ? formatBytes(bytes) : '0 B'
}

export function imageTitle(e: ImageEntry): string | null {
  return e.tags[0] ?? null
}

export function shortId(id: string): string {
  return id.replace(/^sha256:/, '').slice(0, 12)
}

export function uniqueApps(e: ImageEntry): string[] {
  return [...new Set(e.apps.map((a) => a.app))]
}

export function lineageOf(e: ImageEntry) {
  return [...e.apps]
    .filter((a) => a.deploy_id)
    .sort((a, b) => (b.at ?? '').localeCompare(a.at ?? ''))
}
