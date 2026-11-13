import * as React from 'react'
import { PALETTE_DRAG_THRESHOLD_PX } from '@/lib/paletteConfig'
import type { PinnedBubble } from '@/lib/paletteBubbleLayout'
import { pinEntryFor } from '@/lib/paletteBubbleLayout'
import {
  bubbleEls,
  clearShifts,
  insertIndexAt,
  isRtl,
  openGap,
  slotWidth,
} from '@/lib/bubbleMotion'
import type { PaletteItem } from '../components/commandPaletteData'

/** Pointer handlers a result row spreads onto itself to become draggable. */
export interface RowDragProps {
  onPointerDown: (e: React.PointerEvent<HTMLElement>) => void
  onPointerMove: (e: React.PointerEvent<HTMLElement>) => void
  onPointerUp: (e: React.PointerEvent<HTMLElement>) => void
  onPointerCancel: () => void
  /** True once for the click that ends a drag, so it does not run the row. */
  takeClick: () => boolean
}

interface Options {
  rowRef: React.RefObject<HTMLDivElement | null>
  ghostRef: React.RefObject<HTMLDivElement | null>
  onPin: (entry: PinnedBubble, index: number) => void
}

// Drag a page or app result onto the bubble row to pin it. Highlight and gap use refs and CSS variables only.
export function usePinDrop({ rowRef, ghostRef, onPin }: Options) {
  const swallow = React.useRef(false)
  const state = React.useRef<{
    entry: PinnedBubble
    x: number
    started: boolean
    index: number | null
  } | null>(null)

  const clear = () => {
    state.current = null
    const row = rowRef.current
    if (row) {
      row.removeAttribute('data-drop')
      clearShifts(row)
    }
    ghostRef.current?.setAttribute('hidden', '')
  }

  return React.useCallback(
    (item: PaletteItem): RowDragProps | undefined => {
      const entry = pinEntryFor(item)
      if (!entry) return undefined
      return {
        takeClick: () => {
          const was = swallow.current
          swallow.current = false
          return was
        },
        onPointerDown: (e) => {
          // Touch scrolls the list; it uses the keyboard shortcut or the long-press menu instead.
          if (e.button !== 0 || e.pointerType === 'touch') return
          state.current = { entry, x: e.clientX, started: false, index: null }
          e.currentTarget.setPointerCapture?.(e.pointerId)
        },
        onPointerMove: (e) => {
          const s = state.current
          const row = rowRef.current
          if (!s) return
          if (!s.started) {
            if (Math.abs(e.clientX - s.x) < PALETTE_DRAG_THRESHOLD_PX) return
            s.started = true
            const g = ghostRef.current
            if (g) {
              g.textContent = entry.title
              g.removeAttribute('hidden')
            }
          }
          const g = ghostRef.current
          const rtl = row ? isRtl(row) : false
          g?.style.setProperty(
            '--ghost-x',
            `${rtl ? e.clientX - window.innerWidth : e.clientX}px`,
          )
          g?.style.setProperty('--ghost-y', `${e.clientY}px`)
          if (!row) return
          const r = row.getBoundingClientRect()
          const over =
            e.clientX >= r.left &&
            e.clientX <= r.right &&
            e.clientY >= r.top &&
            e.clientY <= r.bottom
          if (over) row.setAttribute('data-drop', 'true')
          else row.removeAttribute('data-drop')
          if (over) {
            const els = bubbleEls(row)
            s.index = insertIndexAt(els, e.clientX, rtl)
            openGap(els, s.index, slotWidth(els), rtl)
          } else {
            s.index = null
            openGap(bubbleEls(row), null, 0, rtl)
          }
        },
        onPointerUp: () => {
          const s = state.current
          if (s?.started) {
            swallow.current = true
            window.setTimeout(() => (swallow.current = false), 0)
            if (s.index !== null) onPin(s.entry, s.index)
          }
          clear()
        },
        onPointerCancel: clear,
      }
    },
    // clear and refs are stable; onPin changes with the layout.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [onPin],
  )
}
