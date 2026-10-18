import type { Tone } from '../kit/tone'
import type { FwAction, FwOutcome } from '../../queries/firewallEdge'

/** Chart stroke per outcome: semantic tokens from index.css, so light and dark never drift. */
export const OUTCOME_STROKE: Record<FwOutcome, string> = {
  allowed: 'var(--tone-info-solid)',
  denied: 'var(--tone-danger-solid)',
  challenged: 'var(--tone-warning-solid)',
  logged: 'var(--tone-neutral-solid)',
  rate_limited: 'var(--chart-series-5)',
}

/** Static class names so Tailwind generates them; mirrors OUTCOME_STROKE for legend swatches. */
export const OUTCOME_SWATCH: Record<FwOutcome, string> = {
  allowed: 'bg-tone-info-solid',
  denied: 'bg-tone-danger-solid',
  challenged: 'bg-tone-warning-solid',
  logged: 'bg-tone-neutral-solid',
  rate_limited: 'bg-[var(--chart-series-5)]',
}

export const ACTION_TONE: Record<FwAction, Tone> = {
  allow: 'success',
  deny: 'danger',
  challenge: 'warning',
  log: 'neutral',
  rate_limit: 'info',
}

const compact = new Intl.NumberFormat('en', {
  notation: 'compact',
  maximumFractionDigits: 1,
})

/** Compact count such as 4.3K; exact below 1000. */
export function formatCount(n: number): string {
  return n < 1000 ? String(n) : compact.format(n)
}

export function sum(values: number[]): number {
  return values.reduce((a, b) => a + b, 0)
}
