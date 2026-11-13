import { describe, expect, it } from 'vitest'
import { computeTraffic } from './appTraffic'
import type { RequestSeries, RequestSummary } from '../types/requests'

const summary = (over: Partial<RequestSummary>): RequestSummary => ({
  window_seconds: 1800,
  has_traffic: true,
  requests: 100,
  rate_per_sec: 0.05,
  error_rate_4xx: 0.02,
  error_rate_5xx: 0.03,
  p95_ms: 412,
  upstream_errors: 0,
  ...over,
})

describe('computeTraffic', () => {
  it('uses the server summary p95 and combines 4xx and 5xx', () => {
    const series: RequestSeries = {
      app: 'web',
      from: '',
      to: '',
      step_seconds: 60,
      summary: summary({}),
      previous_summary: summary({ requests: 50, p95_ms: 300 }),
      points: [
        {
          timestamp: 't',
          requests: 1,
          rate_per_sec: 1,
          error_rate_4xx: 0,
          error_rate_5xx: 0,
          upstream_errors: 0,
          p50_ms: 1,
          p95_ms: 9999,
          p99_ms: 1,
          bytes_in_per_sec: 0,
          bytes_out_per_sec: 0,
        },
      ],
    }
    const t = computeTraffic(series)
    expect(t.current.p95Ms).toBe(412)
    expect(t.current.errorRate).toBeCloseTo(0.05)
    expect(t.previous.requests).toBe(50)
    expect(t.p95Series).toEqual([9999])
  })

  it('falls back to an empty previous window', () => {
    const t = computeTraffic({
      app: 'web',
      from: '',
      to: '',
      step_seconds: 60,
      summary: summary({}),
      points: [],
    })
    expect(t.previous.requests).toBe(0)
    expect(t.hasTraffic).toBe(true)
  })
})
