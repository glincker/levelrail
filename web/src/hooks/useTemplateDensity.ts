import { useCallback, useState } from 'react'

export type TemplateDensity = 'comfortable' | 'compact'

export const DENSITY_STORAGE_KEY = 'templates.density'

function readDensity(): TemplateDensity {
  try {
    return window.localStorage.getItem(DENSITY_STORAGE_KEY) === 'compact'
      ? 'compact'
      : 'comfortable'
  } catch {
    return 'comfortable'
  }
}

/** useTemplateDensity persists the card/list choice; storage failures fall back to in-memory state. */
export function useTemplateDensity(): [
  TemplateDensity,
  (next: TemplateDensity) => void,
] {
  const [density, setDensity] = useState<TemplateDensity>(readDensity)
  const update = useCallback((next: TemplateDensity) => {
    setDensity(next)
    try {
      window.localStorage.setItem(DENSITY_STORAGE_KEY, next)
    } catch {
      // Storage unavailable (private mode, blocked): the choice lasts this session only.
    }
  }, [])
  return [density, update]
}
