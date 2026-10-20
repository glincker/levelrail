import type { Tone } from '../components/kit/tone'
import type { EngineHealth, EngineMetricSeries } from '../types/models'

export function formatEngineMetric(
  value: number,
  unit: EngineMetricSeries['unit'],
): { value: string; unit?: string } {
  switch (unit) {
    case 'percent':
      return { value: value.toFixed(value >= 10 ? 0 : 1), unit: '%' }
    case 'seconds':
      return { value: String(Math.round(value * 1000)), unit: 'ms' }
    case 'tokens_per_second':
      return { value: value.toFixed(1), unit: 'tok/s' }
    case 'bytes':
      return { value: (value / 2 ** 30).toFixed(1), unit: 'GiB' }
    default:
      return { value: String(Math.round(value)) }
  }
}

export function healthTone(health: EngineHealth): Tone {
  switch (health.state) {
    case 'ok':
      return 'success'
    case 'warn':
      return 'warning'
    default:
      return 'neutral'
  }
}

export function healthLabel(health: EngineHealth): string {
  switch (health.state) {
    case 'ok':
      return 'Engine healthy'
    case 'warn':
      return 'Needs attention'
    default:
      return 'No data yet'
  }
}

// Series worth a tile: everything the engine supports, plus the core
// serving metrics shown as "not available" so the gap is explicit.
const CORE_IDS = new Set([
  'kv_cache',
  'queue',
  'prefix_hits',
  'tokens_per_second',
  'ttft',
])

export function visibleSeries(series: EngineMetricSeries[]) {
  return series.filter((s) => s.supported || CORE_IDS.has(s.id))
}

export function seriesTone(s: EngineMetricSeries): Tone {
  if (s.latest === null) return 'neutral'
  if (s.id === 'kv_cache' && s.latest >= 90) return 'warning'
  return 'info'
}
