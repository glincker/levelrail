import { describe, expect, it } from 'vitest'
import {
  errorReading,
  nodeCounts,
  summarizeTraffic,
  type FleetTraffic,
} from './fleetStats'

const traffic = (over: Partial<FleetTraffic>): FleetTraffic => ({
  ratePerSec: 0,
  errorPct: 0,
  series: [],
  reporting: 1,
  ...over,
})

describe('errorReading', () => {
  it.each([
    ['no requests', traffic({ ratePerSec: 0, errorPct: 7.14 }), 'no-traffic'],
    [
      'a trickle below the minimum',
      traffic({ ratePerSec: 0.002, errorPct: 7.14 }),
      'no-traffic',
    ],
    [
      'no app reporting',
      traffic({ reporting: 0, ratePerSec: 5 }),
      'no-traffic',
    ],
    ['enough requests', traffic({ ratePerSec: 0.1, errorPct: 2 }), 'value'],
  ])('%s', (_n, t, kind) => {
    expect(errorReading(t, 20).kind).toBe(kind)
  })

  it('never reports a percentage when the request count is zero', () => {
    const t = summarizeTraffic([
      {
        name: 'a',
        has_traffic: true,
        rate_per_sec: 0,
        error_rate_5xx: 0.0714,
        p95_ms: 0,
        spark: [],
      },
    ])
    expect(errorReading(t, 20).kind).toBe('no-traffic')
  })
})

describe('nodeCounts', () => {
  it('adds the control plane when it has no node row', () => {
    expect(nodeCounts([])).toEqual({ online: 1, total: 1 })
    expect(nodeCounts([{ is_local: false, status: 'offline' }])).toEqual({
      online: 1,
      total: 2,
    })
  })

  it('does not double count a local row', () => {
    expect(
      nodeCounts([
        { is_local: true, status: 'online' },
        { is_local: false, status: 'offline' },
      ]),
    ).toEqual({ online: 1, total: 2 })
  })
})
