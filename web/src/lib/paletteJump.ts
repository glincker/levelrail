import type { PaletteEntry } from '../components/paletteList'

/** Index of the first result under the nth group header (1-based), or null. */
export function firstIndexOfGroup(
  entries: readonly PaletteEntry[],
  n: number,
): number | null {
  let seen = 0
  for (let i = 0; i < entries.length; i++) {
    if (entries[i]?.type !== 'header') continue
    seen++
    if (seen !== n) continue
    const next = entries.slice(i + 1).find((e) => e.type === 'item')
    return next?.type === 'item' ? next.index : null
  }
  return null
}
