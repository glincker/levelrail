import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef, type ReactNode } from 'react'

// Lists past this size render through the virtualizer.
export const VIRTUALIZE_AFTER = 50

/** VirtualRows renders rows directly for short lists and virtualized beyond VIRTUALIZE_AFTER. */
export function VirtualRows<T>({
  items,
  rowHeight,
  getKey,
  renderRow,
  header,
}: {
  items: T[]
  rowHeight: number
  getKey: (item: T) => string
  renderRow: (item: T) => ReactNode
  header: ReactNode
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtual = items.length > VIRTUALIZE_AFTER
  const virtualizer = useVirtualizer({
    count: virtual ? items.length : 0,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 8,
  })

  return (
    <div
      ref={parentRef}
      className="max-h-[70vh] overflow-auto rounded-lg border border-border bg-card"
    >
      {header}
      {virtual ? (
        <div
          className="relative"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((row) => {
            const item = items[row.index]
            if (item === undefined) return null
            return (
              <div
                key={getKey(item)}
                className="absolute top-0 left-0 w-full"
                style={{
                  height: row.size,
                  transform: `translateY(${row.start}px)`,
                }}
              >
                {renderRow(item)}
              </div>
            )
          })}
        </div>
      ) : (
        items.map((item) => <div key={getKey(item)}>{renderRow(item)}</div>)
      )}
    </div>
  )
}
