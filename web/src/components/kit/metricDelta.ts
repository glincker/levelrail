import type { Tone } from './tone'

export interface MetricDelta {
  value: number
  direction: 'up' | 'down'
  goodWhen: 'up' | 'down'
}

export function deltaTone(d: MetricDelta): Tone {
  if (d.value === 0) return 'neutral'
  return d.direction === d.goodWhen ? 'success' : 'danger'
}
