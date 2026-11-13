import { PALETTE_PIN_MAX } from './paletteConfig'

export const BUBBLE_LAYOUT_KEY = 'levelrail:palette-bubbles'
// Written by the docs page pin button; read once to seed the palette, never rewritten here.
export const LEGACY_PINNED_KEY = 'levelrail:pinned-pages'

/** A bubble the user placed by hand. itemKey resolves a palette item, path opens a route. */
export interface PinnedBubble {
  id: string
  title: string
  path?: string
  itemKey?: string
}

function parseList(raw: string | null): unknown[] {
  if (!raw) return []
  const v: unknown = JSON.parse(raw)
  return Array.isArray(v) ? v : []
}

function isPinned(v: unknown): v is PinnedBubble {
  if (typeof v !== 'object' || v === null) return false
  const o = v as Record<string, unknown>
  return typeof o.id === 'string' && typeof o.title === 'string'
}

function dedupe(list: PinnedBubble[]): PinnedBubble[] {
  const seen = new Set<string>()
  return list.filter((p) => !seen.has(p.id) && seen.add(p.id))
}

/** Stored pins, or the legacy page pins when the new key has never been written. */
export function loadPinned(): PinnedBubble[] {
  try {
    const own = localStorage.getItem(BUBBLE_LAYOUT_KEY)
    if (own !== null) {
      return dedupe(parseList(own).filter(isPinned)).slice(0, PALETTE_PIN_MAX)
    }
    const legacy: PinnedBubble[] = []
    for (const v of parseList(localStorage.getItem(LEGACY_PINNED_KEY))) {
      const o = v as { path?: unknown; title?: unknown }
      if (typeof o?.path === 'string' && typeof o.title === 'string') {
        legacy.push({ id: `path:${o.path}`, title: o.title, path: o.path })
      }
    }
    return dedupe(legacy).slice(0, PALETTE_PIN_MAX)
  } catch {
    return []
  }
}

export function savePinned(list: readonly PinnedBubble[]): void {
  try {
    localStorage.setItem(
      BUBBLE_LAYOUT_KEY,
      JSON.stringify(list.slice(0, PALETTE_PIN_MAX)),
    )
  } catch {
    // Storage can be full or blocked; the layout then lasts for the session only.
  }
}

/**
 * Pins after the user placed `moved` in a new order. Everything left of the last
 * placed bubble holds a slot, so the pinned prefix and the display order agree.
 * Returns null when that prefix would pass the pin cap.
 */
export function pinPrefix(
  order: readonly PinnedBubble[],
  pinnedIds: ReadonlySet<string>,
  movedId: string,
): PinnedBubble[] | null {
  let last = -1
  order.forEach((p, i) => {
    if (p.id === movedId || pinnedIds.has(p.id)) last = i
  })
  const next = order.slice(0, last + 1)
  return next.length > PALETTE_PIN_MAX ? null : next
}

/** Moves one item and returns a new array (same array when nothing changes). */
export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  const next = list.slice()
  if (from === to || from < 0 || from >= next.length) return next
  const [item] = next.splice(from, 1)
  if (item === undefined) return next
  next.splice(Math.max(0, Math.min(to, next.length)), 0, item)
  return next
}

/** What pinning a result row stores, or null when it is not a page or an app with a route. */
export function pinEntryFor(item: {
  key: string
  label: string
  preview?: { kind: string; path?: string }
}): PinnedBubble | null {
  const kind = item.preview?.kind
  const path = item.preview?.path
  if (!path || (kind !== 'page' && kind !== 'app')) return null
  return { id: `item:${item.key}`, title: item.label, itemKey: item.key, path }
}
