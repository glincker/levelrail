import type { Tone } from '@/components/kit'

function envNumber(raw: string | undefined, fallback: number): number {
  const n = Number(raw)
  return raw && Number.isFinite(n) && n > 0 ? n : fallback
}

const P95_WARN_MS = envNumber(
  import.meta.env.VITE_P95_WARN_MS as string | undefined,
  500,
)
const P95_CRIT_MS = envNumber(
  import.meta.env.VITE_P95_CRITICAL_MS as string | undefined,
  1500,
)
const ERROR_WARN_PCT = envNumber(
  import.meta.env.VITE_ERROR_RATE_WARN_PERCENT as string | undefined,
  1,
)
const ERROR_CRIT_PCT = envNumber(
  import.meta.env.VITE_ERROR_RATE_CRITICAL_PERCENT as string | undefined,
  5,
)

export function latencyTone(
  ms: number,
  warn = P95_WARN_MS,
  crit = P95_CRIT_MS,
): Tone {
  if (ms >= crit) return 'danger'
  if (ms >= warn) return 'warning'
  return 'neutral'
}

export function errorTone(
  pct: number,
  warn = ERROR_WARN_PCT,
  crit = ERROR_CRIT_PCT,
): Tone {
  if (pct >= crit) return 'danger'
  if (pct >= warn) return 'warning'
  return 'neutral'
}
