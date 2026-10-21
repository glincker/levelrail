import type { DeployFixLink } from './deployFixLinks'

export interface FixReturnMarker {
  appName: string
  deployId: string
  /** Key under deployDetail `fix.link.*`, names what was being fixed. */
  labelKey: DeployFixLink['labelKey']
  /** Epoch milliseconds when the fix link was clicked. */
  at: number
}

const STORAGE_KEY = 'levelrail.fixReturnMarker'
const DEFAULT_TTL_MINUTES = 30

/** Marker lifetime in ms, read from VITE_FIX_BANNER_TTL_MINUTES. */
export function fixMarkerTtlMs(): number {
  const minutes = Number(
    import.meta.env.VITE_FIX_BANNER_TTL_MINUTES ?? DEFAULT_TTL_MINUTES,
  )
  const valid = Number.isFinite(minutes) && minutes > 0
  return (valid ? minutes : DEFAULT_TTL_MINUTES) * 60_000
}

function isMarker(value: unknown): value is FixReturnMarker {
  if (typeof value !== 'object' || value === null) return false
  const v = value as Record<string, unknown>
  return (
    typeof v.appName === 'string' &&
    typeof v.deployId === 'string' &&
    typeof v.labelKey === 'string' &&
    typeof v.at === 'number' &&
    Number.isFinite(v.at)
  )
}

// Storage can be missing or throw (private mode, blocked site data), so
// every access is guarded and the banner simply never shows.
export function writeFixMarker(marker: FixReturnMarker): void {
  try {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(marker))
  } catch {
    // Page works without the marker.
  }
}

export function clearFixMarker(): void {
  try {
    window.sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // Nothing to clear.
  }
}

/** Returns the marker for appName if present, well formed and not expired. */
export function readFixMarker(
  appName: string,
  now: number = Date.now(),
): FixReturnMarker | null {
  let raw: string | null
  try {
    raw = window.sessionStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
  if (!raw) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!isMarker(parsed) || parsed.appName !== appName) return null
  const age = now - parsed.at
  if (age < 0 || age > fixMarkerTtlMs()) return null
  return parsed
}
