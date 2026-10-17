import { useRef, type ReactNode } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { cn } from '@/lib/utils'

/** Lists up to this many rows render plainly; longer ones are virtualized. */
export const VIRTUALIZE_ABOVE = 50

/** VirtualRows renders a divided list and virtualizes it, measuring each row, once it can exceed 50 rows. */
export function VirtualRows<T>({
  items,
  rowHeight,
  label,
  getKey,
  renderRow,
  className,
}: {
  items: T[]
  rowHeight: number
  label: string
  getKey: (item: T) => string
  renderRow: (item: T, index: number) => ReactNode
  className?: string
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtual = items.length > VIRTUALIZE_ABOVE
  const virtualizer = useVirtualizer({
    count: virtual ? items.length : 0,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 8,
  })

  if (!virtual) {
    return (
      <div
        role="list"
        aria-label={label}
        className={cn('divide-y divide-border', className)}
      >
        {items.map((item, i) => (
          <div key={getKey(item)} role="listitem">
            {renderRow(item, i)}
          </div>
        ))}
      </div>
    )
  }

  return (
    <div
      ref={parentRef}
      role="list"
      aria-label={label}
      className={cn('max-h-[28rem] overflow-auto', className)}
    >
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((row) => {
          const item = items[row.index]
          if (item === undefined) return null
          return (
            <div
              key={getKey(item)}
              ref={virtualizer.measureElement}
              data-index={row.index}
              role="listitem"
              className="absolute top-0 left-0 w-full border-b border-border"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              {renderRow(item, row.index)}
            </div>
          )
        })}
      </div>
    </div>
  )
}
