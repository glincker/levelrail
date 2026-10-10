import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, PointerEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CaretUpIcon,
  KeyIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { needsDetail, type ColumnKind } from '../../../lib/explorerData'
import type { DbCell } from '../../../types/databaseViewer'
import { ColumnTypeIcon } from './ColumnTypeIcon'

const ROW_HEIGHT_PX = 32
const OVERSCAN_ROWS = 12
const DEFAULT_WIDTH_PX = 176
const MIN_WIDTH_PX = 72
const KEY_RESIZE_STEP_PX = 16
const SELECT_COLUMN_PX = 40

export interface GridColumn {
  name: string
  type?: string
  kind: ColumnKind
  primaryKey: boolean
}

export interface DataGridProps {
  columns: GridColumn[]
  rows: DbCell[][]
  sort: { column: string; desc: boolean } | null
  onSort: (column: string) => void
  filters: Record<string, string>
  onFilterChange: (column: string, value: string) => void
  selected: ReadonlySet<number>
  onSelectedChange: (next: Set<number>) => void
  activeRow: number
  onActiveRowChange: (row: number) => void
  onOpenRow: (row: number) => void
}

function Cell({
  value,
  kind,
  width,
  onOpen,
}: {
  value: DbCell
  kind: ColumnKind
  width: number
  onOpen: () => void
}) {
  const { t } = useTranslation('databases')
  const base =
    'flex h-8 shrink-0 items-center border-r border-border/60 px-2 text-xs'
  if (value === null) {
    return (
      <div role="gridcell" className={base} style={{ width }}>
        <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium italic text-muted-foreground">
          {t('viewer.grid.null')}
        </span>
      </div>
    )
  }
  if (value === '') {
    return (
      <div role="gridcell" className={base} style={{ width }}>
        <span
          className="font-mono text-muted-foreground"
          title={t('viewer.grid.emptyString')}
        >
          {'""'}
        </span>
      </div>
    )
  }
  const long = needsDetail(value, kind)
  return (
    <div
      role="gridcell"
      className={cn(base, 'font-mono', kind === 'number' && 'justify-end')}
      style={{ width }}
      onDoubleClick={long ? onOpen : undefined}
    >
      <span className="truncate" title={long ? t('viewer.grid.expand') : value}>
        {value.replace(/\s+/g, ' ')}
      </span>
    </div>
  )
}

export function DataGrid({
  columns,
  rows,
  sort,
  onSort,
  filters,
  onFilterChange,
  selected,
  onSelectedChange,
  activeRow,
  onActiveRowChange,
  onOpenRow,
}: DataGridProps) {
  const { t } = useTranslation('databases')
  const scrollRef = useRef<HTMLDivElement>(null)
  const [order, setOrder] = useState<string[]>(() => columns.map((c) => c.name))
  const [widths, setWidths] = useState<Record<string, number>>({})
  const [dragging, setDragging] = useState<string | null>(null)

  const names = columns.map((c) => c.name).join('\u0000')
  useEffect(() => {
    setOrder((cur) => {
      const current = columns.map((c) => c.name)
      const kept = cur.filter((n) => current.includes(n))
      const added = current.filter((n) => !kept.includes(n))
      return [...kept, ...added]
    })
    // columns identity changes every render; the joined names are the key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [names])

  const byName = useMemo(
    () => new Map(columns.map((c, i) => [c.name, { col: c, index: i }])),
    [columns],
  )
  const ordered = order.flatMap((n) => {
    const hit = byName.get(n)
    return hit ? [hit] : []
  })
  const widthOf = (name: string) => widths[name] ?? DEFAULT_WIDTH_PX
  const totalWidth =
    SELECT_COLUMN_PX + ordered.reduce((sum, h) => sum + widthOf(h.col.name), 0)

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: OVERSCAN_ROWS,
  })

  function move(name: string, delta: number) {
    setOrder((cur) => {
      const i = cur.indexOf(name)
      const j = i + delta
      if (i < 0 || j < 0 || j >= cur.length) return cur
      const next = [...cur]
      next.splice(i, 1)
      next.splice(j, 0, name)
      return next
    })
  }

  function dropOn(target: string) {
    if (!dragging || dragging === target) return
    setOrder((cur) => {
      const next = cur.filter((n) => n !== dragging)
      next.splice(next.indexOf(target), 0, dragging)
      return next
    })
    setDragging(null)
  }

  function startResize(e: PointerEvent<HTMLDivElement>, name: string) {
    e.preventDefault()
    const startX = e.clientX
    const startWidth = widthOf(name)
    const onMove = (ev: globalThis.PointerEvent) => {
      const next = Math.max(MIN_WIDTH_PX, startWidth + ev.clientX - startX)
      setWidths((w) => ({ ...w, [name]: next }))
    }
    const onUp = () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  function toggleRow(row: number) {
    const next = new Set(selected)
    if (next.has(row)) next.delete(row)
    else next.add(row)
    onSelectedChange(next)
  }

  function moveActive(next: number) {
    const bounded = Math.max(0, Math.min(rows.length - 1, next))
    onActiveRowChange(bounded)
    virtualizer.scrollToIndex(bounded, { align: 'auto' })
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.target !== e.currentTarget) return
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        moveActive(activeRow + 1)
        break
      case 'ArrowUp':
        e.preventDefault()
        moveActive(activeRow - 1)
        break
      case 'Home':
        e.preventDefault()
        moveActive(0)
        break
      case 'End':
        e.preventDefault()
        moveActive(rows.length - 1)
        break
      case 'Enter':
        e.preventDefault()
        if (rows[activeRow]) onOpenRow(activeRow)
        break
      case ' ':
        e.preventDefault()
        if (rows[activeRow]) toggleRow(activeRow)
        break
      case 'Escape':
        if (selected.size > 0) onSelectedChange(new Set())
        break
      default:
    }
  }

  const allSelected = rows.length > 0 && selected.size === rows.length

  return (
    <div
      ref={scrollRef}
      role="grid"
      tabIndex={0}
      aria-rowcount={rows.length}
      aria-colcount={ordered.length + 1}
      aria-label={t('viewer.grid.label')}
      aria-activedescendant={
        rows[activeRow] ? `grid-row-${activeRow}` : undefined
      }
      onKeyDown={onKeyDown}
      className="max-h-[30rem] overflow-auto rounded-lg border border-border bg-card text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
    >
      <div style={{ width: totalWidth }}>
        <div className="sticky top-0 z-10 bg-muted/90 backdrop-blur">
          <div role="row" className="flex border-b border-border">
            <div
              role="columnheader"
              className="flex h-8 shrink-0 items-center justify-center border-r border-border"
              style={{ width: SELECT_COLUMN_PX }}
            >
              <input
                type="checkbox"
                aria-label={t('viewer.grid.selectAll')}
                checked={allSelected}
                onChange={() => {
                  onSelectedChange(
                    allSelected ? new Set() : new Set(rows.map((_, i) => i)),
                  )
                }}
                className="size-3.5 accent-primary"
              />
            </div>
            {ordered.map(({ col }) => {
              const active = sort?.column === col.name
              return (
                <div
                  key={col.name}
                  role="columnheader"
                  aria-sort={
                    active
                      ? sort.desc
                        ? 'descending'
                        : 'ascending'
                      : undefined
                  }
                  draggable
                  onDragStart={() => {
                    setDragging(col.name)
                  }}
                  onDragOver={(e) => {
                    e.preventDefault()
                  }}
                  onDrop={() => {
                    dropOn(col.name)
                  }}
                  onDragEnd={() => {
                    setDragging(null)
                  }}
                  className={cn(
                    'relative flex h-8 shrink-0 items-center gap-1 border-r border-border px-2',
                    dragging === col.name && 'opacity-50',
                  )}
                  style={{ width: widthOf(col.name) }}
                >
                  <ColumnTypeIcon kind={col.kind} type={col.type} />
                  {col.primaryKey ? (
                    <KeyIcon
                      className="size-3 shrink-0 text-primary"
                      aria-label={t('viewer.explorer.primaryKey')}
                    />
                  ) : null}
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-center gap-1 text-left text-xs font-medium hover:text-foreground"
                    title={t('viewer.grid.headerHint')}
                    onClick={() => {
                      onSort(col.name)
                    }}
                    onKeyDown={(e) => {
                      if (!e.altKey) return
                      if (e.key === 'ArrowLeft') {
                        e.preventDefault()
                        move(col.name, -1)
                      } else if (e.key === 'ArrowRight') {
                        e.preventDefault()
                        move(col.name, 1)
                      }
                    }}
                  >
                    <span className="truncate">{col.name}</span>
                    {active ? (
                      sort.desc ? (
                        <CaretDownIcon className="size-3" aria-hidden="true" />
                      ) : (
                        <CaretUpIcon className="size-3" aria-hidden="true" />
                      )
                    ) : null}
                  </button>
                  <div
                    role="separator"
                    aria-orientation="vertical"
                    aria-label={t('viewer.grid.resize', { column: col.name })}
                    tabIndex={0}
                    onPointerDown={(e) => {
                      startResize(e, col.name)
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
                        e.preventDefault()
                        const delta =
                          e.key === 'ArrowRight'
                            ? KEY_RESIZE_STEP_PX
                            : -KEY_RESIZE_STEP_PX
                        setWidths((w) => ({
                          ...w,
                          [col.name]: Math.max(
                            MIN_WIDTH_PX,
                            widthOf(col.name) + delta,
                          ),
                        }))
                      }
                    }}
                    className="absolute right-0 top-0 h-full w-1.5 cursor-col-resize touch-none hover:bg-primary/40 focus-visible:bg-primary/60 focus-visible:outline-none"
                  />
                </div>
              )
            })}
          </div>
          <div role="row" className="flex border-b border-border bg-card/80">
            <div
              className="shrink-0 border-r border-border"
              style={{ width: SELECT_COLUMN_PX }}
            />
            {ordered.map(({ col }) => (
              <div
                key={col.name}
                role="columnheader"
                className="flex h-8 shrink-0 items-center border-r border-border px-1"
                style={{ width: widthOf(col.name) }}
              >
                <Input
                  value={filters[col.name] ?? ''}
                  placeholder={t('viewer.grid.filterPlaceholder')}
                  aria-label={t('viewer.grid.filterColumn', {
                    column: col.name,
                  })}
                  className="h-6 px-1.5 text-xs"
                  onChange={(e) => {
                    onFilterChange(col.name, e.target.value)
                  }}
                />
              </div>
            ))}
          </div>
        </div>

        <div
          className="relative"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((item) => {
            const row = rows[item.index]
            if (!row) return null
            const isSelected = selected.has(item.index)
            const isActive = item.index === activeRow
            return (
              <div
                key={item.key}
                id={`grid-row-${item.index}`}
                role="row"
                aria-selected={isSelected}
                className={cn(
                  'absolute left-0 top-0 flex w-full border-b border-border/60 hover:bg-muted/40',
                  isSelected && 'bg-primary/10',
                  isActive && 'ring-1 ring-inset ring-ring/60',
                )}
                style={{
                  height: ROW_HEIGHT_PX,
                  transform: `translateY(${item.start}px)`,
                }}
                onClick={() => {
                  onActiveRowChange(item.index)
                }}
                onDoubleClick={() => {
                  onOpenRow(item.index)
                }}
              >
                <div
                  className="flex h-8 shrink-0 items-center justify-center border-r border-border/60"
                  style={{ width: SELECT_COLUMN_PX }}
                >
                  <input
                    type="checkbox"
                    aria-label={t('viewer.grid.selectRow', {
                      row: item.index + 1,
                    })}
                    checked={isSelected}
                    onChange={() => {
                      toggleRow(item.index)
                    }}
                    onClick={(e) => {
                      e.stopPropagation()
                    }}
                    className="size-3.5 accent-primary"
                  />
                </div>
                {ordered.map(({ col, index }) => (
                  <Cell
                    key={col.name}
                    value={row[index] ?? null}
                    kind={col.kind}
                    width={widthOf(col.name)}
                    onOpen={() => {
                      onOpenRow(item.index)
                    }}
                  />
                ))}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
