import { useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'

export const WINDOW_AFTER_ROWS = 50
const DEFAULT_ROW_HEIGHT = 56
const OVERSCAN = 8

/**
 * useWindowedRows windows a plain table past WINDOW_AFTER_ROWS rows using
 * spacer rows, so every list that can grow stays cheap to render. At or
 * below the threshold it returns every index and no spacers.
 */
export function useWindowedRows(count: number, rowHeight = DEFAULT_ROW_HEIGHT) {
  const parentRef = useRef<HTMLDivElement>(null)
  const windowed = count > WINDOW_AFTER_ROWS
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: OVERSCAN,
    enabled: windowed,
  })
  if (!windowed) {
    return {
      parentRef,
      windowed,
      indices: Array.from({ length: count }, (_, i) => i),
      padTop: 0,
      padBottom: 0,
    }
  }
  const items = virtualizer.getVirtualItems()
  const last = items[items.length - 1]
  return {
    parentRef,
    windowed,
    indices: items.map((i) => i.index),
    padTop: items[0]?.start ?? 0,
    padBottom: last ? virtualizer.getTotalSize() - last.end : 0,
  }
}
