import { describe, expect, it } from 'vitest'
import { formatDuration, layoutTrace } from './traceTree'
import type { TraceSpan } from '../types/traces'

const span = (
  id: string,
  parent: string | undefined,
  start: number,
  dur: number,
): TraceSpan => ({
  trace_id: 't',
  span_id: id,
  parent_id: parent,
  name: id,
  kind: 0,
  start_ns: start,
  duration_ns: dur,
  status_code: 0,
})

describe('layoutTrace', () => {
  it('nests children under parents and places them on one axis', () => {
    const { rows, totalNs } = layoutTrace([
      span('db', 'api', 10, 30),
      span('api', undefined, 0, 100),
      span('cache', 'api', 50, 10),
    ])
    expect(totalNs).toBe(100)
    expect(rows.map((r) => [r.span.span_id, r.depth])).toEqual([
      ['api', 0],
      ['db', 1],
      ['cache', 1],
    ])
    expect(rows[1]?.left).toBeCloseTo(0.1)
    expect(rows[1]?.width).toBeCloseTo(0.3)
  })

  it('treats a span with a missing parent as a root', () => {
    const { rows } = layoutTrace([span('orphan', 'gone', 5, 10)])
    expect(rows).toHaveLength(1)
    expect(rows[0]?.depth).toBe(0)
  })

  it('returns nothing for an empty trace', () => {
    expect(layoutTrace([])).toEqual({ rows: [], totalNs: 0 })
  })
})

describe('formatDuration', () => {
  it.each([
    [500, '500ns'],
    [42_000, '42us'],
    [12_300_000, '12.3ms'],
    [2_500_000_000, '2.50s'],
  ])('%d', (ns, want) => {
    expect(formatDuration(ns)).toBe(want)
  })
})
