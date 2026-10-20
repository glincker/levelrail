const DELTA_PERCENT_CAP = 999

// formatDeltaPercent keeps the chip short: one decimal under 10, whole
// numbers above, and a capped label for extreme swings.
export function formatDeltaPercent(value: number): string {
  const abs = Math.abs(value)
  if (!Number.isFinite(abs)) return ''
  if (abs > DELTA_PERCENT_CAP) return `>${DELTA_PERCENT_CAP}%`
  if (abs < 10) return `${Number(abs.toFixed(1))}%`
  return `${Math.round(abs)}%`
}
