import { useEffect, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { APP_LIST_GRID, AppRow } from '../AppRow'
import type { AppListEntry } from '../../types/appDetail'
import type { ViewMode } from '../../lib/appsListView'
import { AppCard } from './AppCard'

export function ListHeader() {
  return (
    <div
      className={`${APP_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium text-muted-foreground`}
    >
      <span aria-hidden="true" />
      <span aria-hidden="true" />
      <span>Name</span>
      <span className="hidden lg:block">Traffic 1h</span>
      <span className="hidden lg:block">p95</span>
      <span className="hidden lg:block">Errors</span>
      <span className="hidden lg:block">Deployed</span>
      <span aria-hidden="true" />
    </div>
  )
}

function columnsForWidth(width: number): number {
  if (width >= 1100) return 3
  if (width >= 700) return 2
  return 1
}

function useColumns(ref: React.RefObject<HTMLDivElement | null>): number {
  const [cols, setCols] = useState(3)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return undefined
    const ro = new ResizeObserver(([entry]) => {
      if (entry) setCols(columnsForWidth(entry.contentRect.width))
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
    }
  }, [ref])
  return cols
}

export function AppsListBody({
  apps,
  mode,
  selected,
  onSelect,
}: {
  apps: AppListEntry[]
  mode: ViewMode
  selected: string[]
  onSelect: (name: string, checked: boolean) => void
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const cols = useColumns(parentRef)
  const perRow = mode === 'grid' ? cols : 1
  const rowCount = Math.ceil(apps.length / perRow)
  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => parentRef.current,
    estimateSize: () => (mode === 'grid' ? 176 : 60),
    overscan: 6,
  })

  return (
    <div
      ref={parentRef}
      className={
        mode === 'grid'
          ? 'h-[70vh] overflow-auto'
          : 'h-[70vh] overflow-auto rounded-2xl border border-border bg-card'
      }
    >
      {mode === 'table' ? <ListHeader /> : null}
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const slice = apps.slice(
            virtualRow.index * perRow,
            virtualRow.index * perRow + perRow,
          )
          return (
            <div
              key={virtualRow.key}
              data-index={virtualRow.index}
              ref={virtualizer.measureElement}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              {mode === 'table' ? (
                slice.map((app) => (
                  <AppRow
                    key={app.name}
                    app={app}
                    selected={selected.includes(app.name)}
                    onSelect={onSelect}
                  />
                ))
              ) : (
                <div
                  className="grid gap-3 pb-3"
                  style={{
                    gridTemplateColumns: `repeat(${perRow}, minmax(0, 1fr))`,
                  }}
                >
                  {slice.map((app) => (
                    <AppCard
                      key={app.name}
                      app={app}
                      selected={selected.includes(app.name)}
                      onSelect={onSelect}
                    />
                  ))}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
