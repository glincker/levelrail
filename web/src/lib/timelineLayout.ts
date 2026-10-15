import {
  barGeometry,
  type BarGeometry,
  type TimelineDeploy,
} from './deployTimeline'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
const TICK_STEPS = [
  MINUTE,
  2 * MINUTE,
  5 * MINUTE,
  10 * MINUTE,
  15 * MINUTE,
  30 * MINUTE,
  HOUR,
  2 * HOUR,
  3 * HOUR,
  6 * HOUR,
  12 * HOUR,
  DAY,
]
const MAX_TICKS = 7

export interface AxisTick {
  at: number
  left: number
  /** True on a local midnight, drawn stronger and labelled with the date. */
  major: boolean
}

function isMidnight(at: number): boolean {
  const d = new Date(at)
  return d.getHours() === 0 && d.getMinutes() === 0
}

/** Round-number ticks inside [start, end], at most MAX_TICKS, aligned to local time. */
export function adaptiveTicks(start: number, end: number): AxisTick[] {
  const span = end - start
  if (span <= 0) return []
  const step =
    TICK_STEPS.find((s) => span / s <= MAX_TICKS) ?? TICK_STEPS.at(-1) ?? DAY
  const s = new Date(start)
  const at: number[] = []
  if (step >= DAY) {
    for (let i = 0; i < 400; i++) {
      const t = new Date(s.getFullYear(), s.getMonth(), s.getDate() + i)
      if (t.getTime() > end) break
      if (t.getTime() >= start) at.push(t.getTime())
    }
  } else {
    const base = new Date(s.getFullYear(), s.getMonth(), s.getDate()).getTime()
    const first = base + Math.ceil((start - base) / step) * step
    for (let t = first; t <= end; t += step) at.push(t)
  }
  return at.map((t) => ({
    at: t,
    left: ((t - start) / span) * 100,
    major: isMidnight(t),
  }))
}

export type MarkerItem =
  | { kind: 'bar'; deploy: TimelineDeploy; geo: BarGeometry; dot: boolean }
  | {
      kind: 'cluster'
      key: string
      deploys: TimelineDeploy[]
      center: number
    }

/** Bars narrower than this percent of the window render as a dot marker. */
export const DOT_WIDTH_PCT = 1
/** Markers closer than this percent of the window collapse into a chip. */
export const CLUSTER_GAP_PCT = 1.6

interface Placed {
  deploy: TimelineDeploy
  geo: BarGeometry
}

/** Places a lane's deploys on the track, merging overlapping or adjacent ones into clusters. */
export function layoutLane(
  deploys: TimelineDeploy[],
  windowStart: number,
  windowEnd: number,
): MarkerItem[] {
  const placed: Placed[] = []
  for (const deploy of deploys) {
    const geo = barGeometry(deploy, windowStart, windowEnd)
    if (geo) placed.push({ deploy, geo })
  }
  placed.sort((a, b) => a.geo.left - b.geo.left)
  const groups: Placed[][] = []
  let right = -Infinity
  for (const p of placed) {
    const last = groups.at(-1)
    if (last && p.geo.left - right <= CLUSTER_GAP_PCT) {
      last.push(p)
      right = Math.max(right, p.geo.left + p.geo.width)
    } else {
      groups.push([p])
      right = p.geo.left + p.geo.width
    }
  }
  return groups.map((g): MarkerItem => {
    const first = g[0]
    if (g.length === 1 && first) {
      return {
        kind: 'bar',
        deploy: first.deploy,
        geo: first.geo,
        dot: first.geo.width < DOT_WIDTH_PCT,
      }
    }
    const from = Math.min(...g.map((p) => p.geo.left))
    const to = Math.max(...g.map((p) => p.geo.left + p.geo.width))
    return {
      kind: 'cluster',
      key: g.map((p) => p.deploy.id).join('|'),
      deploys: g.map((p) => p.deploy),
      center: (from + to) / 2,
    }
  })
}

export interface CardPlacement {
  x: number
  y: number
  below: boolean
}

export const CARD_WIDTH = 288
export const CARD_HEIGHT = 156
const CARD_GAP = 8
const VIEWPORT_PAD = 8

/** Fixed-position card coords: centred over the anchor, flipped below when there is no room above. */
export function cardPlacement(
  anchor: { left: number; right: number; top: number; bottom: number },
  viewport: { width: number; height: number },
): CardPlacement {
  const center = (anchor.left + anchor.right) / 2
  const maxX = Math.max(
    viewport.width - CARD_WIDTH - VIEWPORT_PAD,
    VIEWPORT_PAD,
  )
  const x = Math.min(Math.max(center - CARD_WIDTH / 2, VIEWPORT_PAD), maxX)
  const aboveY = anchor.top - CARD_HEIGHT - CARD_GAP
  const below = aboveY < VIEWPORT_PAD
  return { x, y: below ? anchor.bottom + CARD_GAP : aboveY, below }
}

const DOT_SPACING_PX = 26

/** Horizontal px offset of the index-th of total markers spread evenly around a centre. */
export function spreadOffset(index: number, total: number): number {
  return (index - (total - 1) / 2) * DOT_SPACING_PX
}
