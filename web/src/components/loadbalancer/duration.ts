const UNIT_SECONDS = { ms: 0.001, s: 1, m: 60, h: 3600 } as const

export function parseDuration(value: string): number {
  const m = /^(\d+(?:\.\d+)?)(ms|s|m|h)$/.exec(value.trim())
  if (!m) return Number.NaN
  return Number(m[1]) * UNIT_SECONDS[m[2] as keyof typeof UNIT_SECONDS]
}

export function formatSeconds(seconds: number): string {
  if (!Number.isFinite(seconds)) return '-'
  if (seconds < 1) return `${Math.round(seconds * 1000)}ms`
  if (seconds < 60) return `${Math.round(seconds)}s`
  const minutes = seconds / 60
  return `${Number.isInteger(minutes) ? minutes : minutes.toFixed(1)}m`
}
