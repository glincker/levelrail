import * as React from 'react'
import { flushSync } from 'react-dom'
import { useTranslation } from 'react-i18next'
import {
  PALETTE_DRAG_THRESHOLD_PX,
  PALETTE_LONG_PRESS_MS,
} from '@/lib/paletteConfig'
import { moveItem } from '@/lib/paletteBubbleLayout'
import {
  bubbleEls,
  clearShifts,
  isRtl,
  setShift,
  slotWidth,
} from '@/lib/bubbleMotion'
import type { Bubble } from '../components/palette/RecentBubbles'

interface Press {
  index: number
  startX: number
  pointerId: number
  dragging: boolean
  target: number
  slot: number
  rtl: boolean
  els: HTMLElement[]
}

interface Options {
  bubbles: readonly Bubble[]
  rowRef: React.RefObject<HTMLDivElement | null>
  onReorder?: (next: Bubble[], movedKey: string) => void
  onLongPress: (key: string) => void
}

// Pointer drag, keyboard lift and menu moves for the bubble row. Pointer moves only write CSS variables.
export function useBubbleReorder({
  bubbles,
  rowRef,
  onReorder,
  onLongPress,
}: Options) {
  const { t } = useTranslation('palette')
  const press = React.useRef<Press | null>(null)
  const longPress = React.useRef<number | undefined>(undefined)
  const swallowClick = React.useRef(false)
  const [draft, setDraft] = React.useState<Bubble[] | null>(null)
  const [liftedKey, setLiftedKey] = React.useState<string | null>(null)
  const [announcement, setAnnouncement] = React.useState('')
  const shown = draft ?? bubbles

  const say = (key: string, list: readonly Bubble[], id: string) => {
    const b = list.find((x) => x.key === key)
    setAnnouncement(
      t(id, {
        title: b?.label ?? '',
        position: list.findIndex((x) => x.key === key) + 1,
        total: list.length,
      }),
    )
  }

  const stopLongPress = () => window.clearTimeout(longPress.current)

  function commit(next: Bubble[], movedKey: string) {
    // Flush before resetting the shift variables so the row never flashes the old order.
    flushSync(() => onReorder?.(next, movedKey))
  }

  /** Move a bubble one reading-order slot (-1 earlier, +1 later). Returns false at an edge. */
  const moveBy = (key: string, delta: number): boolean => {
    const i = bubbles.findIndex((b) => b.key === key)
    const to = i + delta
    if (i < 0 || to < 0 || to >= bubbles.length) return false
    const next = moveItem(bubbles, i, to)
    commit(next, key)
    say(key, next, 'bubbles.moved')
    return true
  }

  const onPointerDown = (e: React.PointerEvent<HTMLElement>, i: number) => {
    if (e.button !== 0 || !rowRef.current) return
    const els = bubbleEls(rowRef.current)
    press.current = {
      index: i,
      startX: e.clientX,
      pointerId: e.pointerId,
      dragging: false,
      target: i,
      slot: slotWidth(els),
      rtl: isRtl(rowRef.current),
      els,
    }
    e.currentTarget.setPointerCapture?.(e.pointerId)
    const key = shown[i]?.key
    if (key && e.pointerType !== 'mouse') {
      longPress.current = window.setTimeout(() => {
        swallowClick.current = true
        press.current = null
        onLongPress(key)
      }, PALETTE_LONG_PRESS_MS)
    }
  }

  const onPointerMove = (e: React.PointerEvent<HTMLElement>) => {
    const p = press.current
    if (!p) return
    const dx = e.clientX - p.startX
    if (!p.dragging) {
      if (Math.abs(dx) < PALETTE_DRAG_THRESHOLD_PX) return
      p.dragging = true
      stopLongPress()
      p.els[p.index]?.setAttribute('data-dragging', 'true')
      rowRef.current?.setAttribute('data-reordering', 'true')
    }
    const dir = p.rtl ? -1 : 1
    const target = Math.max(
      0,
      Math.min(
        p.els.length - 1,
        p.index + (p.slot ? Math.round((dx * dir) / p.slot) : 0),
      ),
    )
    p.target = target
    p.els.forEach((el, i) => {
      if (i === p.index) return setShift(el, dx)
      let slots = 0
      if (target > p.index && i > p.index && i <= target) slots = -1
      if (target < p.index && i < p.index && i >= target) slots = 1
      setShift(el, slots * p.slot * dir)
    })
  }

  const finish = (cancel: boolean) => {
    const p = press.current
    press.current = null
    stopLongPress()
    if (!p?.dragging) return
    swallowClick.current = true
    window.setTimeout(() => (swallowClick.current = false), 0)
    const row = rowRef.current
    row?.setAttribute('data-settling', 'true')
    const moved = shown[p.index]
    if (!cancel && moved && p.target !== p.index) {
      const next = moveItem(shown, p.index, p.target)
      commit(next, moved.key)
      say(moved.key, next, 'bubbles.moved')
    }
    if (row) {
      clearShifts(row)
      row.removeAttribute('data-reordering')
      requestAnimationFrame(() => row.removeAttribute('data-settling'))
    }
  }

  /** True when this click is the tail of a drag or long press and must not open the bubble. */
  const takeClick = (): boolean => {
    const was = swallowClick.current
    swallowClick.current = false
    return was
  }

  const lift = (key: string) => {
    setLiftedKey(key)
    setDraft([...bubbles])
    say(key, bubbles, 'bubbles.lifted')
  }
  const drop = (key: string) => {
    if (draft && draft.some((b, i) => b.key !== bubbles[i]?.key)) {
      commit(draft, key)
      say(key, draft, 'bubbles.dropped')
    } else say(key, bubbles, 'bubbles.dropped')
    setLiftedKey(null)
    setDraft(null)
  }
  const cancel = () => {
    setLiftedKey(null)
    setDraft(null)
    setAnnouncement(t('bubbles.cancelled'))
  }

  /** Returns true when the key was handled. */
  const onKeyDown = (e: React.KeyboardEvent<HTMLElement>, key: string) => {
    const rtl = rowRef.current ? isRtl(rowRef.current) : false
    if (e.key === ' ' && !e.repeat) {
      e.preventDefault()
      return liftedKey === key ? drop(key) : lift(key)
    }
    if (liftedKey !== key) return
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      return cancel()
    }
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      e.preventDefault()
      const delta = (e.key === 'ArrowRight' ? 1 : -1) * (rtl ? -1 : 1)
      const list = draft ?? [...bubbles]
      const i = list.findIndex((b) => b.key === key)
      if (i + delta < 0 || i + delta >= list.length) return
      const next = moveItem(list, i, i + delta)
      setDraft(next)
      say(key, next, 'bubbles.moved')
    }
  }

  return {
    shown,
    liftedKey,
    announcement,
    moveBy,
    takeClick,
    cancelLift: () => liftedKey !== null && cancel(),
    onPointerDown,
    onPointerMove,
    onPointerUp: () => finish(false),
    onPointerCancel: () => finish(true),
    onKeyDown,
  }
}
