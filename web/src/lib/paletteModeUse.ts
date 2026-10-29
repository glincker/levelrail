import type { PaletteModeKind } from './paletteMode'

const KEY = 'palette.modeUse'
export const MODE_ORDER: readonly PaletteModeKind[] = [
  'docs',
  'assistant',
  'tags',
  'docker',
]

function read(): Partial<Record<PaletteModeKind, number>> {
  try {
    const raw = window.localStorage.getItem(KEY)
    const parsed: unknown = raw ? JSON.parse(raw) : {}
    const out: Partial<Record<PaletteModeKind, number>> = {}
    if (parsed && typeof parsed === 'object') {
      for (const kind of MODE_ORDER) {
        const n = (parsed as Record<string, unknown>)[kind]
        if (typeof n === 'number' && Number.isFinite(n)) out[kind] = n
      }
    }
    return out
  } catch {
    return {}
  }
}

/** Counts a use of a command mode so the pills can put the favourites first. */
export function recordModeUse(kind: PaletteModeKind): void {
  try {
    const use = read()
    use[kind] = (use[kind] ?? 0) + 1
    window.localStorage.setItem(KEY, JSON.stringify(use))
  } catch {
    // storage unavailable: the order just stays default
  }
}

/** Modes by use count, most used first; ties keep the default order. */
export function modesByUse(): PaletteModeKind[] {
  const use = read()
  return [...MODE_ORDER].sort(
    (a, b) =>
      (use[b] ?? 0) - (use[a] ?? 0) ||
      MODE_ORDER.indexOf(a) - MODE_ORDER.indexOf(b),
  )
}
