import { describe, expect, it } from 'vitest'
import { rollingDrainOrder } from './rollingDrain'

const node = (id: string, workloads: number, online = true) => ({
  id,
  online,
  schedulable: true,
  workloads,
})

describe('rollingDrainOrder', () => {
  it('orders the zone by fewest workloads first', () => {
    const r = rollingDrainOrder([node('a', 5), node('b', 1)], [node('c', 0)], 1)
    expect(r).toEqual({ ok: true, order: ['b', 'a'] })
  })

  it('refuses below min available and on an empty zone', () => {
    expect(rollingDrainOrder([node('a', 1)], [node('c', 0, false)], 1)).toEqual(
      { ok: false, reason: 'minAvailable', stay: 0 },
    )
    expect(rollingDrainOrder([], [node('c', 0)], 1).ok).toBe(false)
  })
})
