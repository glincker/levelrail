import { useState } from 'react'
import type { ReactNode } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { cn } from '@/lib/utils'

const VIRTUALIZE_AFTER = 50
const ROW_HEIGHT_PX = 56

/** VirtualRows renders every row up to 50 items and windows the list past that, so a long principal list stays cheap. */
export function VirtualRows<T>({
  items,
  rowKey,
  renderRow,
  label,
  className,
  maxHeightClass = 'max-h-96',
}: {
  items: T[]
  rowKey: (item: T) => string
  renderRow: (item: T) => ReactNode
  label: string
  className?: string
  maxHeightClass?: string
}) {
  const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null)
  const virtualize = items.length > VIRTUALIZE_AFTER
  const virtualizer = useVirtualizer({
    count: virtualize ? items.length : 0,
    getScrollElement: () => scrollEl,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 8,
  })

  if (!virtualize) {
    return (
      <ul
        aria-label={label}
        className={cn(
          'divide-y divide-border overflow-y-auto rounded-lg border border-border',
          maxHeightClass,
          className,
        )}
      >
        {items.map((item) => (
          <li key={rowKey(item)}>{renderRow(item)}</li>
        ))}
      </ul>
    )
  }

  return (
    <div
      ref={setScrollEl}
      role="list"
      aria-label={label}
      className={cn(
        'overflow-y-auto rounded-lg border border-border',
        maxHeightClass,
        className,
      )}
    >
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((v) => {
          const item = items[v.index]
          if (item === undefined) return null
          return (
            <div
              key={rowKey(item)}
              role="listitem"
              className="absolute inset-x-0 border-b border-border"
              style={{ height: v.size, transform: `translateY(${v.start}px)` }}
            >
              {renderRow(item)}
            </div>
          )
        })}
      </div>
    </div>
  )
}
