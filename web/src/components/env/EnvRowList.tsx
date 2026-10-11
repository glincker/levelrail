import { Fragment, useRef, type ReactNode } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { EnvRow } from '../../lib/envRows'

const VIRTUALIZE_AFTER = 50
const ESTIMATED_ROW_PX = 64

export interface EnvRowListProps {
  rows: EnvRow[]
  label: string
  renderRow: (row: EnvRow) => ReactNode
}

function VirtualRows({ rows, label, renderRow }: EnvRowListProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_ROW_PX,
    overscan: 8,
  })
  return (
    <div ref={scrollRef} className="max-h-[70vh] overflow-auto">
      <div
        role="list"
        aria-label={label}
        className="relative"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((item) => {
          const row = rows[item.index]
          if (!row) return null
          return (
            <div
              key={row.id}
              data-index={item.index}
              ref={virtualizer.measureElement}
              className="absolute inset-x-0 border-b border-border"
              style={{ transform: `translateY(${String(item.start)}px)` }}
            >
              {renderRow(row)}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// Long lists are virtualized; short ones render plainly so tests and
// screen readers see every row.
export function EnvRowList(props: EnvRowListProps) {
  if (props.rows.length > VIRTUALIZE_AFTER) {
    return <VirtualRows {...props} />
  }
  return (
    <div
      role="list"
      aria-label={props.label}
      className="divide-y divide-border"
    >
      {props.rows.map((row) => (
        <Fragment key={row.id}>{props.renderRow(row)}</Fragment>
      ))}
    </div>
  )
}
