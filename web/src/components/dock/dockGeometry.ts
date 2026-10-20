export type DockMode = 'float' | 'bottom' | 'side'
export type DockView = 'open' | 'minimized' | 'closed'
export type Corner = 'br' | 'bl' | 'tr' | 'tl'
export type DockLayout = DockMode | 'sheet'

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface Viewport {
  w: number
  h: number
}

export const MOBILE_QUERY = '(max-width: 767px)'
export const EDGE_GAP = 16
export const FLOAT_MIN_W = 320
export const FLOAT_MIN_H = 280
export const FLOAT_DEFAULT_W = 420
export const BOTTOM_MIN = 200
export const SIDE_MIN = 320
export const LAUNCHER_SIZE = 56
export const KEY_STEP = 16
export const KEY_STEP_LARGE = 64

export function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), Math.max(min, max))
}

export function viewportClass(vw: number): 'md' | 'lg' | 'xl' {
  if (vw < 1024) return 'md'
  if (vw < 1536) return 'lg'
  return 'xl'
}

export function maxBottom(vp: Viewport): number {
  return Math.max(BOTTOM_MIN, Math.floor(vp.h * 0.8))
}

export function maxSide(vp: Viewport): number {
  return Math.max(SIDE_MIN, Math.floor(vp.w * 0.6))
}

export function maximizedFloat(vp: Viewport): Rect {
  return {
    x: EDGE_GAP / 2,
    y: EDGE_GAP / 2,
    w: Math.max(FLOAT_MIN_W, vp.w - EDGE_GAP),
    h: Math.max(FLOAT_MIN_H, vp.h - EDGE_GAP),
  }
}

export function clampFloat(r: Rect, vp: Viewport): Rect {
  const w = clamp(r.w, FLOAT_MIN_W, vp.w - EDGE_GAP)
  const h = clamp(r.h, FLOAT_MIN_H, vp.h - EDGE_GAP)
  return {
    w,
    h,
    x: clamp(r.x, 0, vp.w - w),
    y: clamp(r.y, 0, vp.h - h),
  }
}

// `avoid` lifts the default corner above fixed bottom bars already on screen.
export function defaultFloat(vp: Viewport, avoid: number): Rect {
  const w = Math.min(FLOAT_DEFAULT_W, vp.w - EDGE_GAP)
  const h = Math.max(
    FLOAT_MIN_H,
    Math.min(640, Math.floor(vp.h * 0.7), vp.h - EDGE_GAP * 2 - avoid),
  )
  return clampFloat(
    { x: vp.w - w - EDGE_GAP, y: vp.h - h - EDGE_GAP - avoid, w, h },
    vp,
  )
}

export function cornerPosition(
  corner: Corner,
  vp: Viewport,
  avoid: number,
): { x: number; y: number } {
  const right = vp.w - LAUNCHER_SIZE - EDGE_GAP
  const bottom = vp.h - LAUNCHER_SIZE - EDGE_GAP - avoid
  return {
    x: corner === 'br' || corner === 'tr' ? right : EDGE_GAP,
    y: corner === 'br' || corner === 'bl' ? bottom : EDGE_GAP,
  }
}

export function nearestCorner(cx: number, cy: number, vp: Viewport): Corner {
  const right = cx > vp.w / 2
  const bottom = cy > vp.h / 2
  if (bottom) return right ? 'br' : 'bl'
  return right ? 'tr' : 'tl'
}

export type FloatEdge = 'l' | 't' | 'b' | 'bl'

export function resizeFloat(
  edge: FloatEdge,
  start: Rect,
  dx: number,
  dy: number,
  vp: Viewport,
): Rect {
  const maxW = vp.w - EDGE_GAP
  const maxH = vp.h - EDGE_GAP
  let { x, y, w, h } = start
  if (edge === 'l' || edge === 'bl') {
    w = clamp(start.w - dx, FLOAT_MIN_W, Math.min(maxW, start.x + start.w))
    x = start.x + start.w - w
  }
  if (edge === 't') {
    h = clamp(start.h - dy, FLOAT_MIN_H, Math.min(maxH, start.y + start.h))
    y = start.y + start.h - h
  }
  if (edge === 'b' || edge === 'bl') {
    h = clamp(start.h + dy, FLOAT_MIN_H, Math.min(maxH, vp.h - start.y))
  }
  return { x, y, w, h }
}
