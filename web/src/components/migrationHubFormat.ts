const UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB']

export function formatBytes(n: number): string {
  if (n < 0) return '?'
  let v = n
  let i = 0
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024
    i++
  }
  return `${i === 0 ? v : v.toFixed(1)} ${UNITS[i]}`
}

export function formatSeconds(sec: number): string {
  if (sec < 90) return `${sec} s`
  if (sec < 5400) return `${Math.round(sec / 60)} min`
  return `${(sec / 3600).toFixed(1)} h`
}

export const HUB_STEPS = [
  'connect',
  'inventory',
  'preflight',
  'copy',
  'verify',
  'cutover',
] as const

export type HubViewStep = (typeof HUB_STEPS)[number]
