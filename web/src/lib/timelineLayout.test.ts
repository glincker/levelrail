import { describe, expect, it } from 'vitest'
import type { TimelineDeploy } from './deployTimeline'
import {
  adaptiveTicks,
  cardPlacement,
  CARD_WIDTH,
  layoutLane,
} from './timelineLayout'

const HOUR = 3_600_000
const WS = new Date(2026, 9, 14, 0, 0, 0).getTime()

function deploy(id: string, startMin: number, durMin: number): TimelineDeploy {
  const s = WS + startMin * 60_000
  return {
    id,
    app: 'web',
    status: 'ready',
    trigger: '',
    environment: '',
    commit_sha: '',
    commit_message: '',
    author: '',
    strategy: '',
    project_id: '',
    node_id: '',
    started_at: new Date(s).toISOString(),
    finished_at: new Date(s + durMin * 60_000).toISOString(),
    duration_ms: durMin * 60_000,
    rollback_of: null,
    is_live: false,
  }
}

describe('adaptiveTicks', () => {
  it('uses round steps and marks day boundaries', () => {
    const ticks = adaptiveTicks(WS - 6 * HOUR, WS + 18 * HOUR)
    expect(ticks.length).toBeLessThanOrEqual(7)
    expect(ticks.length).toBeGreaterThan(2)
    const major = ticks.filter((t) => t.major)
    expect(major).toHaveLength(1)
    expect(major[0]?.at).toBe(WS)
    for (const t of ticks) expect(new Date(t.at).getMinutes()).toBe(0)
  })

  it('ticks once per day over a week', () => {
    const ticks = adaptiveTicks(WS - 3 * 24 * HOUR - HOUR, WS + 4 * 24 * HOUR)
    expect(ticks.every((t) => t.major)).toBe(true)
    expect(ticks).toHaveLength(8)
  })

  it('uses minute steps for an hour and stays in range', () => {
    const ticks = adaptiveTicks(WS + 7 * 60_000, WS + 67 * 60_000)
    expect(ticks.length).toBeLessThanOrEqual(7)
    for (const t of ticks) {
      expect(t.left).toBeGreaterThanOrEqual(0)
      expect(t.left).toBeLessThanOrEqual(100)
    }
  })

  it('returns nothing for an empty window', () => {
    expect(adaptiveTicks(WS, WS)).toEqual([])
  })
})

describe('layoutLane', () => {
  const end = WS + 24 * HOUR

  it('keeps distant deploys separate and flags short ones as dots', () => {
    const items = layoutLane(
      [deploy('a', 60, 0.2), deploy('b', 600, 120)],
      WS,
      end,
    )
    expect(items.map((i) => i.kind)).toEqual(['bar', 'bar'])
    const [a, b] = items
    expect(a?.kind === 'bar' && a.dot).toBe(true)
    expect(b?.kind === 'bar' && b.dot).toBe(false)
  })

  it('clusters adjacent and overlapping deploys', () => {
    const items = layoutLane(
      [deploy('a', 60, 1), deploy('b', 62, 1), deploy('c', 63, 30)],
      WS,
      end,
    )
    expect(items).toHaveLength(1)
    const [c] = items
    expect(c?.kind === 'cluster' && c.deploys.map((d) => d.id)).toEqual([
      'a',
      'b',
      'c',
    ])
  })

  it('drops deploys outside the window', () => {
    expect(layoutLane([deploy('x', -500, 1)], WS, end)).toEqual([])
  })
})

describe('cardPlacement', () => {
  const vp = { width: 1000, height: 800 }

  it('sits above the anchor when there is room', () => {
    const p = cardPlacement(
      { left: 400, right: 420, top: 400, bottom: 420 },
      vp,
    )
    expect(p.below).toBe(false)
    expect(p.y).toBeLessThan(400)
  })

  it('flips below near the top and clamps horizontally', () => {
    const p = cardPlacement({ left: 0, right: 10, top: 20, bottom: 40 }, vp)
    expect(p.below).toBe(true)
    expect(p.x).toBe(8)
    const q = cardPlacement({ left: 990, right: 1000, top: 20, bottom: 40 }, vp)
    expect(q.x).toBe(1000 - CARD_WIDTH - 8)
  })
})
