import { describe, expect, it } from 'vitest'
import { computeTraffic } from './appTraffic'
import type { RequestPoint } from '../types/requests'

const NOW = Date.parse('2026-01-01T01:00:00Z')

function point(minutesAgo: number, requests: number): RequestPoint {
  return {
    timestamp: new Date(NOW - minutesAgo * 60_000).toISOString(),
    requests,
    rate_per_sec: requests / 60,
    error_rate_4xx: 0,
    error_rate_5xx: 0,
    p50_ms: 10,
    p95_ms: 20,
    p99_ms: 30,
  } as RequestPoint
}

describe('computeTraffic', () => {
  it('divides by the window length so idle minutes do not inflate the rate', () => {
    const stats = computeTraffic([point(5, 600)], 60, NOW)
    expect(stats.current.ratePerSec).toBeCloseTo(600 / 3600)
  })
})
