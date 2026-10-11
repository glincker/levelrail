import { useRef, type ReactNode } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'

const VIRTUALIZE_AFTER = 50

// Plain list up to 50 rows, windowed beyond that so a long drain or
// connectivity matrix never renders every row.
export function VirtualRows<T>({
  items,
  rowKey,
  renderRow,
  estimateSize = 64,
}: {
  items: T[]
  rowKey: (item: T) => string
  renderRow: (item: T) => ReactNode
  estimateSize?: number
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualized = items.length > VIRTUALIZE_AFTER
  const virtualizer = useVirtualizer({
    count: virtualized ? items.length : 0,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan: 6,
  })

  if (!virtualized) {
    return (
      <ul className="divide-y divide-border rounded-lg border border-border">
        {items.map((item) => (
          <li key={rowKey(item)}>{renderRow(item)}</li>
        ))}
      </ul>
    )
  }

  return (
    <div
      ref={parentRef}
      className="max-h-96 overflow-auto rounded-lg border border-border"
    >
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((row) => {
          const item = items[row.index]
          if (!item) {
            return null
          }
          return (
            <div
              key={rowKey(item)}
              data-index={row.index}
              ref={virtualizer.measureElement}
              className="absolute top-0 left-0 w-full border-b border-border"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              {renderRow(item)}
            </div>
          )
        })}
      </div>
    </div>
  )
}
