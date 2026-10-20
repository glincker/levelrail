import { useEffect, useLayoutEffect, useMemo, useRef } from 'react'
import type { RefObject } from 'react'
import { useMediaQuery } from '../../hooks/useMediaQuery'
import {
  BOTTOM_MIN,
  EDGE_GAP,
  FLOAT_MIN_H,
  FLOAT_MIN_W,
  MOBILE_QUERY,
  SIDE_MIN,
  clamp,
  clampFloat,
  defaultFloat,
  maxBottom,
  maxSide,
  maximizedFloat,
  resizeFloat,
  viewportClass,
  type DockLayout,
  type FloatEdge,
  type Rect,
} from './dockGeometry'
import {
  resetDockGeometry,
  setDockBottom,
  setDockFloat,
  setDockSide,
  useDockState,
} from './dockStore'
import { useBottomAvoid, useViewport } from './useDockViewport'

const ZERO: Rect = { x: 0, y: 0, w: 0, h: 0 }
const PUSH_B = '--dock-push-b'
const PUSH_R = '--dock-push-r'

export interface HandleBounds {
  now: number
  min: number
  max: number
}

function applyBox(el: HTMLElement | null, layout: DockLayout, box: Rect) {
  if (!el) return
  const s = el.style
  s.transform = ''
  s.width = ''
  s.height = ''
  if (layout === 'float') {
    s.transform = `translate3d(${String(box.x)}px,${String(box.y)}px,0)`
    s.width = `${String(box.w)}px`
    s.height = `${String(box.h)}px`
  } else if (layout === 'bottom') {
    s.height = `${String(box.h)}px`
  } else if (layout === 'side') {
    s.width = `${String(box.w)}px`
  }
}

function applyPush(active: boolean, layout: DockLayout, box: Rect) {
  const root = document.documentElement.style
  root.removeProperty(PUSH_B)
  root.removeProperty(PUSH_R)
  if (!active) return
  if (layout === 'bottom') root.setProperty(PUSH_B, `${String(box.h)}px`)
  if (layout === 'side') root.setProperty(PUSH_R, `${String(box.w)}px`)
}

// Owns where the dock sits and how it is moved or resized. Gestures write the
// DOM directly (transform and size) and persist once, on release.
export function useDockBox(ref: RefObject<HTMLElement | null>) {
  const state = useDockState()
  const vp = useViewport()
  const avoid = useBottomAvoid()
  const mobile = useMediaQuery(MOBILE_QUERY)
  const layout: DockLayout = mobile ? 'sheet' : state.mode
  const sizeClass = viewportClass(vp.w)
  const saved = state.floats[sizeClass]

  const box = useMemo<Rect>(() => {
    if (layout === 'float') {
      if (state.maximized) return maximizedFloat(vp)
      return clampFloat(saved ?? defaultFloat(vp, avoid), vp)
    }
    if (layout === 'bottom') {
      const h = state.maximized
        ? maxBottom(vp)
        : clamp(state.bottom, BOTTOM_MIN, maxBottom(vp))
      return { ...ZERO, h }
    }
    if (layout === 'side') {
      const w = state.maximized
        ? maxSide(vp)
        : clamp(state.side, SIDE_MIN, maxSide(vp))
      return { ...ZERO, w }
    }
    return ZERO
  }, [layout, state.maximized, state.bottom, state.side, saved, vp, avoid])

  const live = useRef<Rect>(box)
  const start = useRef<Rect>(box)
  const pushActive = state.push && state.view === 'open'

  useLayoutEffect(() => {
    live.current = box
    applyBox(ref.current, layout, box)
    applyPush(pushActive, layout, box)
  }, [box, layout, pushActive, ref, state.view])

  useEffect(
    () => () => {
      applyPush(false, 'float', ZERO)
    },
    [],
  )

  function preview(next: Rect) {
    live.current = next
    applyBox(ref.current, layout, next)
    applyPush(pushActive, layout, next)
  }

  function persist(next: Rect) {
    if (layout === 'float') setDockFloat(sizeClass, next)
    else if (layout === 'bottom') setDockBottom(next.h)
    else if (layout === 'side') setDockSide(next.w)
  }

  function beginGesture() {
    start.current = box
    live.current = box
    ref.current?.setAttribute('data-dragging', 'true')
  }

  function endGesture() {
    ref.current?.removeAttribute('data-dragging')
    persist(live.current)
  }

  function moveBy(dx: number, dy: number) {
    const s = start.current
    preview(clampFloat({ ...s, x: s.x + dx, y: s.y + dy }, vp))
  }

  function nudge(dx: number, dy: number) {
    persist(clampFloat({ ...box, x: box.x + dx, y: box.y + dy }, vp))
  }

  function resizeBy(edge: FloatEdge, dx: number, dy: number) {
    const s = start.current
    if (layout === 'float') return preview(resizeFloat(edge, s, dx, dy, vp))
    if (layout === 'bottom') {
      const h = clamp(s.h - dy, BOTTOM_MIN, maxBottom(vp))
      return preview({ ...s, h })
    }
    if (layout === 'side') {
      preview({ ...s, w: clamp(s.w - dx, SIDE_MIN, maxSide(vp)) })
    }
  }

  // Keyboard resize: a positive amount grows the dock.
  function stepBy(edge: FloatEdge, amount: number) {
    if (layout === 'float') {
      const dx = edge === 'l' || edge === 'bl' ? -amount : 0
      const dy =
        edge === 't' ? -amount : edge === 'b' || edge === 'bl' ? amount : 0
      return persist(resizeFloat(edge, box, dx, dy, vp))
    }
    if (layout === 'bottom') {
      return persist({
        ...box,
        h: clamp(box.h + amount, BOTTOM_MIN, maxBottom(vp)),
      })
    }
    if (layout === 'side') {
      persist({ ...box, w: clamp(box.w + amount, SIDE_MIN, maxSide(vp)) })
    }
  }

  function cancelGesture() {
    ref.current?.removeAttribute('data-dragging')
  }

  function resetPosition() {
    resetDockGeometry(sizeClass)
  }

  function bounds(edge: FloatEdge): HandleBounds {
    const vertical = edge === 'l'
    if (layout === 'bottom') {
      return { now: box.h, min: BOTTOM_MIN, max: maxBottom(vp) }
    }
    if (layout === 'side')
      return { now: box.w, min: SIDE_MIN, max: maxSide(vp) }
    return vertical
      ? { now: box.w, min: FLOAT_MIN_W, max: vp.w - EDGE_GAP }
      : { now: box.h, min: FLOAT_MIN_H, max: vp.h - EDGE_GAP }
  }

  return {
    layout,
    box,
    vp,
    sizeClass,
    beginGesture,
    endGesture,
    moveBy,
    nudge,
    resizeBy,
    stepBy,
    cancelGesture,
    resetPosition,
    bounds,
  }
}
