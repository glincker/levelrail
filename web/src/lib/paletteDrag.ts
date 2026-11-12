export interface Offset {
  x: number
  y: number
}
export interface Box {
  left: number
  top: number
  width: number
  height: number
}
export interface Viewport {
  width: number
  height: number
}

/** Least width of the pill that must stay on screen: the grip plus a share of the pill. */
export function minVisibleWidth(
  pillWidth: number,
  grip: number,
  divisor: number,
): number {
  return grip + pillWidth / divisor
}

interface ClampInput {
  /** The palette's box with no offset applied. */
  base: Box
  /** Just the search pill, used for vertical reach and the minimum-visible rule. */
  pill: Box
  viewport: Viewport
  minVisible: number
  rtl: boolean
}

/**
 * Keeps the palette reachable but lets it sit flush against any border. A palette that fits is kept
 * fully inside horizontally; a wider one may overflow as long as the grip and part of the pill show.
 */
export function clampOffset(o: Offset, c: ClampInput): Offset {
  const { base, pill, viewport: vp } = c
  let minX: number
  let maxX: number
  if (base.width <= vp.width) {
    minX = -base.left
    maxX = vp.width - base.width - base.left
  } else if (c.rtl) {
    minX = c.minVisible - (base.left + base.width) - 0
    maxX = vp.width - (base.left + base.width)
    minX = Math.min(minX, maxX)
  } else {
    minX = -base.left
    maxX = vp.width - c.minVisible - base.left
  }
  const minY = -pill.top
  const maxY = vp.height - pill.height - pill.top
  return {
    x: Math.min(Math.max(o.x, minX), Math.max(minX, maxX)),
    y: Math.min(Math.max(o.y, minY), Math.max(minY, maxY)),
  }
}

function nearest(value: number, targets: number[], dist: number): number {
  let best = value
  let bestD = dist + 1
  for (const t of targets) {
    const d = Math.abs(t - value)
    if (d <= dist && d < bestD) {
      best = t
      bestD = d
    }
  }
  return best
}

/** Eases toward an edge, corner or the centre line when within `dist` of it. */
export function snapOffset(
  o: Offset,
  base: Box,
  pill: Box,
  vp: Viewport,
  dist: number,
): Offset {
  const left = base.left + o.x
  const top = pill.top + o.y
  const targetsX = [0, vp.width - base.width, (vp.width - base.width) / 2]
  const targetsY = [0, vp.height - pill.height, (vp.height - pill.height) / 2]
  return {
    x: o.x + (nearest(left, targetsX, dist) - left),
    y: o.y + (nearest(top, targetsY, dist) - top),
  }
}

/** True when the pill sits in the lower half, so the results should open upward. */
export function shouldFlipUp(pill: Box, offset: Offset, vp: Viewport): boolean {
  return pill.top + offset.y + pill.height / 2 > vp.height / 2
}

/** Space left for the results in the chosen direction. */
export function availableSpace(
  pill: Box,
  offset: Offset,
  vp: Viewport,
  up: boolean,
): number {
  const top = pill.top + offset.y
  return up ? top : vp.height - (top + pill.height)
}

/** Arrow key to offset delta; Shift jumps to the next edge, expressed as a large step. */
export function nudge(
  key: string,
  step: number,
  jump: boolean,
  vp: Viewport,
): Offset | null {
  const big = Math.max(vp.width, vp.height)
  const s = jump ? big : step
  switch (key) {
    case 'ArrowLeft':
      return { x: -s, y: 0 }
    case 'ArrowRight':
      return { x: s, y: 0 }
    case 'ArrowUp':
      return { x: 0, y: -s }
    case 'ArrowDown':
      return { x: 0, y: s }
    default:
      return null
  }
}
