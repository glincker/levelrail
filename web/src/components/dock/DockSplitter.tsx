import * as React from 'react'
import { cn } from '@/lib/utils'
import { KEY_STEP, KEY_STEP_LARGE, type FloatEdge } from './dockGeometry'
import type { HandleBounds } from './useDockBox'
import { useDragMove } from './useDragMove'

interface Props {
  edge: FloatEdge
  label: string
  bounds: HandleBounds
  className?: string
  onStart: () => void
  onResize: (dx: number, dy: number) => void
  onEnd: () => void
  onStep: (amount: number) => void
}

// WAI-ARIA window splitter: a focusable separator that exposes its value.
export function DockSplitter({
  edge,
  label,
  bounds,
  className,
  onStart,
  onResize,
  onEnd,
  onStep,
}: Props) {
  const drag = useDragMove({
    enabled: true,
    onStart,
    onDrag: onResize,
    onEnd,
  })
  const vertical = edge === 'l'
  const corner = edge === 'bl'
  const cursor = corner
    ? 'cursor-nesw-resize'
    : vertical
      ? 'cursor-col-resize'
      : 'cursor-row-resize'

  function onKeyDown(e: React.KeyboardEvent) {
    const step = e.shiftKey ? KEY_STEP_LARGE : KEY_STEP
    const grow =
      edge === 'l' ? 'ArrowLeft' : edge === 't' ? 'ArrowUp' : 'ArrowDown'
    const shrink =
      edge === 'l' ? 'ArrowRight' : edge === 't' ? 'ArrowDown' : 'ArrowUp'
    const amounts: Record<string, number> = {
      [grow]: step,
      [shrink]: -step,
      Home: bounds.min - bounds.now,
      End: bounds.max - bounds.now,
    }
    const amount = amounts[e.key]
    if (amount === undefined) return
    e.preventDefault()
    e.stopPropagation()
    if (amount !== 0) onStep(amount)
  }

  return (
    <div
      {...(corner
        ? { 'aria-hidden': true }
        : {
            role: 'separator',
            tabIndex: 0,
            'aria-label': label,
            'aria-orientation': vertical ? 'vertical' : 'horizontal',
            'aria-valuenow': Math.round(bounds.now),
            'aria-valuemin': bounds.min,
            'aria-valuemax': Math.round(bounds.max),
          })}
      onKeyDown={corner ? undefined : onKeyDown}
      {...drag}
      className={cn(
        'absolute z-10 touch-none bg-transparent transition-colors duration-150 outline-none hover:bg-border focus-visible:bg-ring motion-reduce:transition-none',
        cursor,
        className,
      )}
    />
  )
}
