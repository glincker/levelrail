export function formatHitRate(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

export function formatCount(n: number): string {
  return new Intl.NumberFormat().format(Math.round(n))
}

export function formatRate(perSec: number): string {
  return perSec >= 10 ? perSec.toFixed(0) : perSec.toFixed(2)
}
