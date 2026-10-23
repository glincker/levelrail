export const NAV_PINS_STORAGE_KEY = 'shell.nav.global.pins'

const DEFAULT_MAX_PINS = 5

export function maxNavPins(
  raw: unknown = import.meta.env.VITE_NAV_MAX_PINS,
): number {
  const n = Number(raw)
  return Number.isInteger(n) && n > 0 ? n : DEFAULT_MAX_PINS
}

// Keeps only unique strings, in stored order, up to the cap. Anything else
// (bad JSON, wrong shape) yields no pins so a corrupt value never breaks the sidebar.
export function parsePins(raw: string | null, max = maxNavPins()): string[] {
  if (!raw) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const out: string[] = []
    for (const v of parsed) {
      if (typeof v === 'string' && v !== '' && !out.includes(v)) out.push(v)
    }
    return out.slice(0, max)
  } catch {
    return []
  }
}

export function addPin(
  pins: readonly string[],
  id: string,
  max = maxNavPins(),
): string[] {
  if (pins.includes(id) || pins.length >= max) return [...pins]
  return [...pins, id]
}

export function removePin(pins: readonly string[], id: string): string[] {
  return pins.filter((p) => p !== id)
}

export function togglePin(
  pins: readonly string[],
  id: string,
  max = maxNavPins(),
): string[] {
  return pins.includes(id) ? removePin(pins, id) : addPin(pins, id, max)
}

export function readPins(storage?: Pick<Storage, 'getItem'>): string[] {
  try {
    return parsePins(
      (storage ?? window.localStorage).getItem(NAV_PINS_STORAGE_KEY),
    )
  } catch {
    return []
  }
}

export function writePins(
  pins: readonly string[],
  storage?: Pick<Storage, 'setItem'>,
): void {
  try {
    ;(storage ?? window.localStorage).setItem(
      NAV_PINS_STORAGE_KEY,
      JSON.stringify(pins),
    )
  } catch {
    // Storage unavailable: pins last for this session only.
  }
}
