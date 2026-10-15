import type { BrandIconName } from '../components/BrandIcon'

export interface DeviceMarks {
  browser: BrandIconName | null
  os: BrandIconName | null
  kind: 'browser' | 'cli' | 'unknown'
}

const BROWSERS: ReadonlyArray<readonly [string, BrandIconName]> = [
  ['edge', 'edge'],
  ['opera', 'opera'],
  ['brave', 'brave'],
  ['firefox', 'firefox'],
  ['chrome', 'chrome'],
  ['safari', 'safari'],
]

const SYSTEMS: ReadonlyArray<readonly [string, BrandIconName]> = [
  ['windows', 'windows'],
  ['ios', 'apple'],
  ['macos', 'apple'],
  ['android', 'android'],
  ['linux', 'linux'],
]

/** deviceMarks reads the server's fixed "Browser on OS" label into brand marks. */
export function deviceMarks(label: string): DeviceMarks {
  const lower = label.toLowerCase()
  if (lower.includes('command line') || lower.includes('curl')) {
    return {
      browser: lower.includes('curl') ? 'curl' : null,
      os: null,
      kind: 'cli',
    }
  }
  const browser = BROWSERS.find(([k]) => lower.includes(k))?.[1] ?? null
  const os = SYSTEMS.find(([k]) => lower.includes(` on ${k}`))?.[1] ?? null
  return { browser, os, kind: browser || os ? 'browser' : 'unknown' }
}

/** lastSeenTone buckets how recently a session was used for the status dot. */
export function lastSeenTone(
  iso: string,
  now: number,
): 'live' | 'recent' | 'idle' {
  const age = now - new Date(iso).getTime()
  if (age < 5 * 60_000) return 'live'
  if (age < 60 * 60_000) return 'recent'
  return 'idle'
}
