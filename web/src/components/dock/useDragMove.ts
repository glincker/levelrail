import { useRef } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'

const DRAG_THRESHOLD = 4
const INTERACTIVE = 'button,a,input,textarea,select,[role="tab"],[role="menu"]'

interface Options {
  enabled: boolean
  allowInteractive?: boolean
  onStart?: () => void
  onDrag: (dx: number, dy: number) => void
  onEnd: (dx: number, dy: number, moved: boolean) => void
}

interface Session {
  x: number
  y: number
  moved: boolean
  pointerId: number
}

// Pointer-capture drag that works for mouse, pen and touch.
export function useDragMove({
  enabled,
  allowInteractive = false,
  onStart,
  onDrag,
  onEnd,
}: Options) {
  const session = useRef<Session | null>(null)

  function onPointerDown(e: ReactPointerEvent<HTMLElement>) {
    if (!enabled || e.button !== 0) return
    if (!allowInteractive && (e.target as HTMLElement).closest(INTERACTIVE)) {
      return
    }
    session.current = {
      x: e.clientX,
      y: e.clientY,
      moved: false,
      pointerId: e.pointerId,
    }
    e.currentTarget.setPointerCapture(e.pointerId)
    onStart?.()
  }

  function onPointerMove(e: ReactPointerEvent<HTMLElement>) {
    const s = session.current
    if (!s || s.pointerId !== e.pointerId) return
    const dx = e.clientX - s.x
    const dy = e.clientY - s.y
    if (!s.moved && Math.hypot(dx, dy) < DRAG_THRESHOLD) return
    s.moved = true
    onDrag(dx, dy)
  }

  function finish(e: ReactPointerEvent<HTMLElement>) {
    const s = session.current
    if (!s || s.pointerId !== e.pointerId) return
    session.current = null
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId)
    }
    onEnd(e.clientX - s.x, e.clientY - s.y, s.moved)
  }

  return {
    onPointerDown,
    onPointerMove,
    onPointerUp: finish,
    onPointerCancel: finish,
  }
}
