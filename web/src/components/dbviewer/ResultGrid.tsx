import { useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronUpGlassIcon,
  CopyGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { CodeBlock } from '@/components/ui/code-block'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { inferColumnType } from '../../lib/sqlConsole/inferType'
import type { DbCell } from '../../types/databaseViewer'

const ROW_HEIGHT_PX = 32
const OVERSCAN_ROWS = 12
const MAX_CELL_CHARS = 80
const DEFAULT_COL_PX = 192
const MIN_COL_PX = 64
const MAX_COL_PX = 800
const KEY_RESIZE_PX = 16

export interface ResultGridProps {
  columns: string[]
  rows: DbCell[][]
  sortColumn?: string
  sortDesc?: boolean
  onSort?: (column: string) => void
  className?: string
  // Console extras: draggable column edges, type chips, copy-cell buttons.
  enhanced?: boolean
}

interface Expanded {
  column: string
  value: string
}

// Virtualized result grid shared by the table viewer and the SQL console.
// Rows are windowed, so a 1000-row result costs the same to render as 20.
export function ResultGrid({
  columns,
  rows,
  sortColumn,
  sortDesc,
  onSort,
  className,
  enhanced = false,
}: ResultGridProps) {
  const { t } = useTranslation('databases')
  const scrollRef = useRef<HTMLDivElement>(null)
  const [expanded, setExpanded] = useState<Expanded | null>(null)
  const [widths, setWidths] = useState<Record<number, number>>({})
  const types = useMemo(
    () => (enhanced ? columns.map((_, i) => inferColumnType(rows, i)) : []),
    [enhanced, columns, rows],
  )
  const widthOf = (i: number) => widths[i] ?? DEFAULT_COL_PX
  const clamp = (n: number) => Math.min(MAX_COL_PX, Math.max(MIN_COL_PX, n))

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: OVERSCAN_ROWS,
  })

  return (
    <>
      <div
        ref={scrollRef}
        className={cn(
          'max-h-[28rem] overflow-auto rounded-lg border border-border bg-card text-sm',
          className,
        )}
      >
        <div className="min-w-max">
          <div
            className="sticky top-0 z-10 flex border-b border-border bg-muted/80 backdrop-blur"
            role="row"
          >
            {columns.map((column, ci) => {
              const active = sortColumn === column
              return (
                <div
                  key={column}
                  role="columnheader"
                  aria-sort={
                    active ? (sortDesc ? 'descending' : 'ascending') : undefined
                  }
                  className={cn(
                    'relative flex h-8 shrink-0 items-center border-r border-border px-2 last:border-r-0',
                    enhanced ? '' : 'w-48',
                  )}
                  style={enhanced ? { width: widthOf(ci) } : undefined}
                >
                  {onSort ? (
                    <button
                      type="button"
                      className="flex w-full items-center gap-1 truncate text-left font-medium hover:text-foreground"
                      title={
                        active && !sortDesc
                          ? t('viewer.grid.sortDescending')
                          : t('viewer.grid.sortAscending')
                      }
                      onClick={() => {
                        onSort(column)
                      }}
                    >
                      <span className="truncate">{column}</span>
                      {active ? (
                        sortDesc ? (
                          <ChevronDownGlassIcon
                            className="size-4.5"
                            aria-hidden="true"
                          />
                        ) : (
                          <ChevronUpGlassIcon
                            className="size-4.5"
                            aria-hidden="true"
                          />
                        )
                      ) : null}
                    </button>
                  ) : (
                    <span className="truncate font-medium">{column}</span>
                  )}
                  {enhanced ? (
                    <>
                      <span
                        className="ml-1 shrink-0 rounded bg-muted px-1 font-mono text-xs text-muted-foreground"
                        title={t('viewer.console.results.typeInferred')}
                      >
                        {t(
                          `viewer.console.results.types.${types[ci] ?? 'text'}`,
                        )}
                      </span>
                      <div
                        role="separator"
                        aria-orientation="vertical"
                        tabIndex={0}
                        aria-label={t('viewer.console.results.resizeColumn', {
                          column,
                        })}
                        aria-valuenow={widthOf(ci)}
                        aria-valuemin={MIN_COL_PX}
                        aria-valuemax={MAX_COL_PX}
                        className="absolute inset-y-0 right-0 w-1.5 cursor-col-resize hover:bg-primary/40 focus-visible:bg-primary/60 focus-visible:outline-none"
                        onPointerDown={(e) => {
                          e.preventDefault()
                          const startX = e.clientX
                          const startW = widthOf(ci)
                          const move = (ev: PointerEvent) => {
                            setWidths((w) => ({
                              ...w,
                              [ci]: clamp(startW + ev.clientX - startX),
                            }))
                          }
                          const up = () => {
                            window.removeEventListener('pointermove', move)
                            window.removeEventListener('pointerup', up)
                          }
                          window.addEventListener('pointermove', move)
                          window.addEventListener('pointerup', up)
                        }}
                        onKeyDown={(e) => {
                          const d =
                            e.key === 'ArrowRight'
                              ? KEY_RESIZE_PX
                              : e.key === 'ArrowLeft'
                                ? -KEY_RESIZE_PX
                                : 0
                          if (d !== 0) {
                            e.preventDefault()
                            setWidths((w) => ({
                              ...w,
                              [ci]: clamp(widthOf(ci) + d),
                            }))
                          }
                        }}
                      />
                    </>
                  ) : null}
                </div>
              )
            })}
          </div>
          <div
            className="relative"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((item) => {
              const row = rows[item.index]
              if (!row) {
                return null
              }
              return (
                <div
                  key={item.key}
                  role="row"
                  className="absolute left-0 top-0 flex w-full border-b border-border/60 hover:bg-muted/40"
                  style={{
                    height: ROW_HEIGHT_PX,
                    transform: `translateY(${item.start}px)`,
                  }}
                >
                  {columns.map((column, ci) => (
                    <GridCell
                      key={column + String(ci)}
                      value={row[ci] ?? null}
                      widthPx={enhanced ? widthOf(ci) : undefined}
                      copyable={enhanced}
                      onExpand={(value) => {
                        setExpanded({ column, value })
                      }}
                    />
                  ))}
                </div>
              )
            })}
          </div>
        </div>
      </div>

      <Dialog
        open={expanded !== null}
        onOpenChange={(open) => {
          if (!open) {
            setExpanded(null)
          }
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {t('viewer.grid.cellTitle', { column: expanded?.column ?? '' })}
            </DialogTitle>
            <DialogDescription>
              {t('viewer.grid.rowCount', { count: rows.length })}
            </DialogDescription>
          </DialogHeader>
          <CodeBlock
            code={expanded?.value ?? ''}
            wrap
            maxHeight="lg"
            wrapToggle={false}
          />
        </DialogContent>
      </Dialog>
    </>
  )
}

function GridCell({
  value,
  onExpand,
  widthPx,
  copyable,
}: {
  value: DbCell
  onExpand: (value: string) => void
  widthPx?: number
  copyable?: boolean
}) {
  const { t } = useTranslation('databases')
  const sizing = widthPx === undefined ? 'w-48' : ''
  const sizeStyle = widthPx === undefined ? undefined : { width: widthPx }
  if (value === null) {
    return (
      <div
        className={cn(
          'flex h-8 shrink-0 items-center border-r border-border/60 px-2 italic text-muted-foreground last:border-r-0',
          sizing,
        )}
        style={sizeStyle}
      >
        {t('viewer.grid.null')}
      </div>
    )
  }
  const long = value.length > MAX_CELL_CHARS || value.includes('\n')
  const shown = long
    ? value.slice(0, MAX_CELL_CHARS).replace(/\n/g, ' ')
    : value
  return (
    <div
      className={cn(
        'group/cell relative flex h-8 shrink-0 items-center border-r border-border/60 px-2 font-mono text-xs last:border-r-0',
        sizing,
      )}
      style={sizeStyle}
    >
      {long ? (
        <button
          type="button"
          className="w-full truncate text-left hover:text-primary"
          title={t('viewer.grid.expand')}
          onClick={() => {
            onExpand(value)
          }}
        >
          {shown}
        </button>
      ) : (
        <span className="truncate">{shown}</span>
      )}
      {copyable ? (
        <button
          type="button"
          className="absolute right-1 top-1/2 hidden size-6 -translate-y-1/2 items-center justify-center rounded bg-card text-muted-foreground hover:text-foreground focus-visible:flex group-focus-within/cell:flex group-hover/cell:flex"
          title={t('viewer.console.results.copyCell')}
          aria-label={t('viewer.console.results.copyCell')}
          onClick={() => {
            navigator.clipboard.writeText(value).then(
              () => {
                toast.add({
                  title: t('viewer.console.results.copied'),
                  type: 'success',
                })
              },
              () => {
                toast.add({
                  title: t('viewer.console.results.copyFailed'),
                  type: 'error',
                })
              },
            )
          }}
        >
          <CopyGlassIcon className="size-4" aria-hidden="true" />
        </button>
      ) : null}
    </div>
  )
}
