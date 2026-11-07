import type { VitalsMetric, VitalsRating } from '../queries/appVitals'

// Core Web Vitals "good" and "poor" boundaries are fixed public standards (ms, CLS unitless).
export const VITALS_THRESHOLDS: Record<
  VitalsMetric,
  { good: number; poor: number }
> = {
  LCP: { good: 2500, poor: 4000 },
  INP: { good: 200, poor: 500 },
  CLS: { good: 0.1, poor: 0.25 },
  TTFB: { good: 800, poor: 1800 },
  FCP: { good: 1800, poor: 3000 },
}

export function rateVital(metric: VitalsMetric, value: number): VitalsRating {
  const t = VITALS_THRESHOLDS[metric]
  if (value <= t.good) return 'good'
  if (value <= t.poor) return 'needs-improvement'
  return 'poor'
}

export const RATING_TONE: Record<VitalsRating, string> = {
  good: 'text-tone-success',
  'needs-improvement': 'text-tone-warning',
  poor: 'text-tone-danger',
}
