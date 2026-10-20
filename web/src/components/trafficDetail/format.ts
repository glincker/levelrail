export function formatPercent(fraction: number): string {
  return `${(fraction * 100).toFixed(2)}%`
}

export function formatMs(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)} s`
  return `${Math.round(ms)} ms`
}

export function formatRate(perSec: number): string {
  return perSec >= 10 ? perSec.toFixed(0) : perSec.toFixed(2)
}
