import type { ImageEntry } from '../../queries/imageInventory'
import { imageTitle, shortId } from './imageFormat'

export const IMAGE_FILTERS = [
  'all',
  'platform',
  'other',
  'unused',
  'in_use',
] as const

export type ImageFilter = (typeof IMAGE_FILTERS)[number]
export type ImageSort = 'size' | 'age'

export function parseImageFilter(v: unknown): ImageFilter {
  return IMAGE_FILTERS.find((f) => f === v) ?? 'all'
}

function matchesFilter(i: ImageEntry, f: ImageFilter): boolean {
  switch (f) {
    case 'platform':
      return !i.unmanaged
    case 'other':
      return i.unmanaged
    case 'unused':
      return !i.in_use
    case 'in_use':
      return i.in_use
    default:
      return true
  }
}

export function filterImages(
  images: ImageEntry[],
  filter: ImageFilter,
  query: string,
  sort: ImageSort,
): ImageEntry[] {
  const q = query.trim().toLowerCase()
  const out = images.filter((i) => {
    if (!matchesFilter(i, filter)) return false
    if (q === '') return true
    const hay = [imageTitle(i) ?? '', ...i.tags, i.id, shortId(i.id)]
    return hay.some((h) => h.toLowerCase().includes(q))
  })
  return out.sort((a, b) =>
    sort === 'size'
      ? b.size_bytes - a.size_bytes
      : b.created_at.localeCompare(a.created_at),
  )
}

// Bytes freed by removing a set, net of layers shared with images that stay.
export function freedBytes(images: ImageEntry[]): number {
  return images.reduce(
    (sum, i) => sum + Math.max(i.size_bytes - i.shared_bytes, 0),
    0,
  )
}
