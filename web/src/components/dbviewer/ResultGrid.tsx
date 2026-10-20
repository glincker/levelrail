import { useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CaretUpIcon,
  CopyIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import type { DbCell } from '../../types/databaseViewer'

const ROW_HEIGHT_PX = 32
const OVERSCAN_ROWS = 12
const MAX_CELL_CHARS = 80

export interface ResultGridProps {
  columns: string[]
  rows: DbCell[][]
  sortColumn?: string
  sortDesc?: boolean
  onSort?: (column: string) => void
  className?: string
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
}: ResultGridProps) {
  const { t } = useTranslation('databases')
  const scrollRef = useRef<HTMLDivElement>(null)
  const [expanded, setExpanded] = useState<Expanded | null>(null)

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: OVERSCAN_ROWS,
  })

  function copy(value: string) {
    void navigator.clipboard
      .writeText(value)
      .then(() => {
        toast.add({ title: t('viewer.grid.copied'), type: 'success' })
      })
      .catch(() => undefined)
  }

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
            {columns.map((column) => {
              const active = sortColumn === column
              return (
                <div
                  key={column}
                  role="columnheader"
                  aria-sort={
                    active ? (sortDesc ? 'descending' : 'ascending') : undefined
                  }
                  className="flex h-8 w-48 shrink-0 items-center border-r border-border px-2 last:border-r-0"
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
                          <CaretDownIcon
                            className="size-3"
                            aria-hidden="true"
                          />
                        ) : (
                          <CaretUpIcon className="size-3" aria-hidden="true" />
                        )
                      ) : null}
                    </button>
                  ) : (
                    <span className="truncate font-medium">{column}</span>
                  )}
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
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-3 font-mono text-xs">
            {expanded?.value}
          </pre>
          <div className="flex justify-end">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                copy(expanded?.value ?? '')
              }}
            >
              <CopyIcon className="size-3.5" aria-hidden="true" />
              {t('viewer.grid.copy')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

function GridCell({
  value,
  onExpand,
}: {
  value: DbCell
  onExpand: (value: string) => void
}) {
  const { t } = useTranslation('databases')
  if (value === null) {
    return (
      <div className="flex h-8 w-48 shrink-0 items-center border-r border-border/60 px-2 italic text-muted-foreground last:border-r-0">
        {t('viewer.grid.null')}
      </div>
    )
  }
  const long = value.length > MAX_CELL_CHARS || value.includes('\n')
  const shown = long
    ? value.slice(0, MAX_CELL_CHARS).replace(/\n/g, ' ')
    : value
  return (
    <div className="flex h-8 w-48 shrink-0 items-center border-r border-border/60 px-2 font-mono text-xs last:border-r-0">
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
    </div>
  )
}
