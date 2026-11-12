import * as React from 'react'
import {
  PALETTE_GRIP_PX,
  PALETTE_MIN_VISIBLE_DIVISOR,
  PALETTE_NUDGE_PX,
  PALETTE_SNAP_PX,
} from '@/lib/paletteConfig'
import {
  availableSpace,
  clampOffset,
  minVisibleWidth,
  nudge,
  shouldFlipUp,
  snapOffset,
  type Box,
  type Offset,
} from '@/lib/paletteDrag'

const STORAGE_KEY = 'palette.position'
const ANNOUNCE_EVERY_MS = 1000
const SNAP_MS = 150
const ORIGIN: Offset = { x: 0, y: 0 }

function load(): Offset {
  try {
    const raw = window.sessionStorage.getItem(STORAGE_KEY)
    const parsed: unknown = raw ? JSON.parse(raw) : null
    if (parsed && typeof parsed === 'object') {
      const { x, y } = parsed as Partial<Offset>
      if (typeof x === 'number' && typeof y === 'number') return { x, y }
    }
  } catch {
    // Session storage unavailable: the palette just starts centred.
  }
  return ORIGIN
}

function save(o: Offset) {
  try {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(o))
  } catch {
    // Not remembered, harmless.
  }
}

function box(el: Element, o: Offset): Box {
  const r = el.getBoundingClientRect()
  return {
    left: r.left - o.x,
    top: r.top - o.y,
    width: r.width,
    height: r.height,
  }
}

interface Options {
  enabled: boolean
  rtl: boolean
  reducedMotion: boolean
  /** Describes the new position for the polite live region. */
  describe: (o: Offset) => string
}

// Moves the palette with a transform only: offsets live in CSS variables set directly on the popup, so a drag never re-renders React.
export function usePaletteDrag({
  enabled,
  rtl,
  reducedMotion,
  describe,
}: Options) {
  const popup = React.useRef<HTMLElement | null>(null)
  const offset = React.useRef<Offset>(ORIGIN)
  const drag = React.useRef<{
    id: number
    px: number
    py: number
    start: Offset
  } | null>(null)
  const lastAnnounce = React.useRef(0)
  const geometry = React.useRef<{ base: Box; pill: Box } | null>(null)
  const [announcement, setAnnouncement] = React.useState('')

  const measure = React.useCallback(() => {
    const el = popup.current
    const field = el?.querySelector('[data-slot="command-palette-field"]')
    if (!el || !field) return null
    const vp = { width: window.innerWidth, height: window.innerHeight }
    // Mid-ease rects lag the target, so reuse the last settled geometry until the ease ends.
    const settled = el.dataset.snapping ? geometry.current : null
    const base = settled?.base ?? box(el, offset.current)
    const pill = settled?.pill ?? box(field, offset.current)
    geometry.current = { base, pill }
    const min = minVisibleWidth(
      pill.width,
      PALETTE_GRIP_PX,
      PALETTE_MIN_VISIBLE_DIVISOR,
    )
    return { el, vp, base, pill, min }
  }, [])

  const apply = React.useCallback((o: Offset) => {
    offset.current = o
    popup.current?.style.setProperty('--pal-x', `${o.x}px`)
    popup.current?.style.setProperty('--pal-y', `${o.y}px`)
  }, [])

  // Chooses the opening direction and the room for the results, once per move rather than per pointer event.
  const settle = React.useCallback(() => {
    const m = measure()
    if (!m) return
    const up = shouldFlipUp(m.pill, offset.current, m.vp)
    m.el.dataset.flip = up ? 'up' : 'down'
    // Results start at the popup edge (below the mode row and any wrapped bubbles), not at the search field.
    const room = availableSpace(m.base, offset.current, m.vp, up)
    m.el.style.setProperty('--pal-maxh', `${Math.max(room - 16, 160)}px`)
  }, [measure])

  const clamp = React.useCallback(
    (o: Offset) => {
      const m = measure()
      if (!m) return o
      return clampOffset(o, {
        base: m.base,
        pill: m.pill,
        viewport: m.vp,
        minVisible: m.min,
        rtl,
      })
    },
    [measure, rtl],
  )

  const move = React.useCallback(
    (o: Offset, animate: boolean) => {
      const el = popup.current
      if (!el) return
      const eased = animate && !reducedMotion
      if (eased) el.dataset.snapping = 'true'
      apply(o)
      save(o)
      // Measuring mid-transition would read the old position, so settle once the ease has finished.
      window.setTimeout(
        () => {
          delete el.dataset.snapping
          settle()
        },
        eased ? SNAP_MS + 30 : 0,
      )
    },
    [apply, reducedMotion, settle],
  )

  const reset = React.useCallback(() => move(ORIGIN, true), [move])

  const attach = React.useCallback(
    (node: HTMLElement | null) => {
      popup.current = node
      if (!node) return
      node.dataset.flip = 'down'
      apply(enabled ? clamp(load()) : ORIGIN)
      settle()
    },
    [apply, clamp, enabled, settle],
  )

  // Phones keep dragging off but still resize the results when the screen rotates.
  React.useEffect(() => {
    const onResize = () => {
      if (enabled) apply(clamp(offset.current))
      settle()
    }
    window.addEventListener('resize', onResize)
    window.addEventListener('orientationchange', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      window.removeEventListener('orientationchange', onResize)
    }
  }, [enabled, apply, clamp, settle])

  const announce = React.useCallback(
    (o: Offset) => {
      const now = Date.now()
      if (now - lastAnnounce.current < ANNOUNCE_EVERY_MS) return
      lastAnnounce.current = now
      setAnnouncement(describe(o))
    },
    [describe],
  )

  const gripProps = React.useMemo(
    () => ({
      onPointerDown: (e: React.PointerEvent<HTMLElement>) => {
        if (!enabled || e.button !== 0) return
        try {
          e.currentTarget.setPointerCapture(e.pointerId)
        } catch {
          // Capture can fail for synthetic pointers; the drag still follows move events.
        }
        drag.current = {
          id: e.pointerId,
          px: e.clientX,
          py: e.clientY,
          start: offset.current,
        }
        if (popup.current) popup.current.dataset.dragging = 'true'
      },
      onPointerMove: (e: React.PointerEvent<HTMLElement>) => {
        const d = drag.current
        if (!d || d.id !== e.pointerId) return
        apply(
          clamp({
            x: d.start.x + e.clientX - d.px,
            y: d.start.y + e.clientY - d.py,
          }),
        )
      },
      onPointerUp: (e: React.PointerEvent<HTMLElement>) => {
        const d = drag.current
        if (!d || d.id !== e.pointerId) return
        drag.current = null
        if (popup.current) delete popup.current.dataset.dragging
        const m = measure()
        const landed =
          m && !e.altKey
            ? clamp(
                snapOffset(
                  offset.current,
                  m.base,
                  m.pill,
                  m.vp,
                  PALETTE_SNAP_PX,
                ),
              )
            : offset.current
        move(landed, true)
      },
      onPointerCancel: () => {
        drag.current = null
        if (popup.current) delete popup.current.dataset.dragging
      },
      onDoubleClick: () => reset(),
      onKeyDown: (e: React.KeyboardEvent<HTMLElement>) => {
        if (!enabled) return
        if (e.key === 'Home') {
          e.preventDefault()
          reset()
          announce(ORIGIN)
          return
        }
        if (!e.altKey) return
        const m = measure()
        const delta = m && nudge(e.key, PALETTE_NUDGE_PX, e.shiftKey, m.vp)
        if (!delta) return
        e.preventDefault()
        const next = clamp({
          x: offset.current.x + delta.x,
          y: offset.current.y + delta.y,
        })
        move(next, true)
        announce(next)
      },
    }),
    [enabled, apply, clamp, measure, move, reset, announce],
  )

  return { attach, gripProps, announcement }
}
