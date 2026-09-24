import { describe, expect, it } from 'vitest'
import type { DeployAttempt } from '../types/deployAttempt'
import {
  bucketRestarts,
  buildHealthTimeline,
  crashloopWindows,
  failedDeployWindows,
  positionOf,
} from './healthTimeline'

const MIN = 60 * 1000
const from = Date.parse('2026-09-01T00:00:00Z')
const to = from + 24 * 60 * MIN

function attempt(over: Partial<DeployAttempt>): DeployAttempt {
  return {
    id: 'abcdef123456',
    service_name: 'web',
    image: 'img',
    status: 'succeeded',
    started_at: new Date(from + 60 * MIN).toISOString(),
    ...over,
  }
}

describe('positionOf', () => {
  it.each([
    [from, 0],
    [to, 1],
    [from + 12 * 60 * MIN, 0.5],
    [from - MIN, 0],
    [to + MIN, 1],
  ])('maps %d to %d', (t, want) => {
    expect(positionOf(t, from, to)).toBeCloseTo(want)
  })
  it('returns 0 for an empty range', () => {
    expect(positionOf(5, 10, 10)).toBe(0)
  })
})

describe('bucketRestarts', () => {
  it('groups restarts in the same bucket and drops out-of-range ones', () => {
    const out = bucketRestarts(
      [from + MIN, from + 2 * MIN, to - MIN, from - MIN, to + MIN],
      from,
      to,
    )
    expect(out.map((b) => b.count)).toEqual([2, 1])
    expect(out[0]?.label).toContain('2 container restarts')
    expect(out[1]?.label).toContain('1 container restart ')
  })
  it('returns nothing for no data', () => {
    expect(bucketRestarts([], from, to)).toEqual([])
  })
})

describe('crashloopWindows', () => {
  const cases: [string, number[], number][] = [
    ['two restarts is not a crashloop', [from + MIN, from + 2 * MIN], 0],
    [
      'three close restarts is one',
      [from + MIN, from + 2 * MIN, from + 3 * MIN],
      1,
    ],
    [
      'a wide gap splits clusters',
      [
        from + MIN,
        from + 2 * MIN,
        from + 3 * MIN,
        from + 200 * MIN,
        from + 201 * MIN,
        from + 202 * MIN,
      ],
      2,
    ],
  ]
  it.each(cases)('%s', (_n, times, want) => {
    expect(crashloopWindows(times, from, to)).toHaveLength(want)
  })
})

describe('failedDeployWindows', () => {
  it('only windows failed attempts and enforces a minimum width', () => {
    const out = failedDeployWindows(
      [attempt({ id: 'ok' }), attempt({ id: 'bad', status: 'failed' })],
      from,
      to,
    )
    expect(out).toHaveLength(1)
    expect(out[0]?.endPos).toBeGreaterThan(out[0]?.startPos ?? 1)
  })
  it('skips windows fully outside the range', () => {
    const old = attempt({
      status: 'failed',
      started_at: new Date(from - 600 * MIN).toISOString(),
      finished_at: new Date(from - 500 * MIN).toISOString(),
    })
    expect(failedDeployWindows([old], from, to)).toEqual([])
  })
})

describe('buildHealthTimeline', () => {
  it('assembles deploys, restarts and windows', () => {
    const tl = buildHealthTimeline({
      attempts: [attempt({ id: 'a' }), attempt({ id: 'b', status: 'failed' })],
      restartTimes: [from + MIN, from + 2 * MIN, from + 3 * MIN],
      from,
      to,
    })
    expect(tl.deploys).toHaveLength(2)
    expect(tl.restarts).toHaveLength(1)
    expect(tl.windows.map((w) => w.kind).sort()).toEqual([
      'crashloop',
      'failed_deploy',
    ])
  })
})
