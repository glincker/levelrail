import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useDatabaseTablePage } from '../../queries/databaseViewer'
import {
  columnKind,
  dialectOf,
  parseFilterInput,
  toCsv,
  toInsert,
  toJson,
} from '../../lib/explorerData'
import type {
  DbColumn,
  DbColumnFilter,
  DbPageParams,
  DbViewerLimits,
} from '../../types/databaseViewer'
import { DataGrid, type GridColumn } from './explorer/DataGrid'
import { RowDetailDrawer } from './explorer/RowDetailDrawer'
import { useCopy } from './explorer/useCopy'
import { formatSize } from '../../lib/format'

const PAGE_SIZE = 100
const FILTER_DEBOUNCE_MS = 350

// Paginated, sortable, filterable view of one table. Sorting and
// filtering run in the database; column names are validated server-side
// against the table's own catalog.
export function TableDataView({
  databaseName,
  engine,
  schema,
  table,
  columns: meta,
  limits,
}: {
  databaseName: string
  engine: string
  schema: string
  table: string
  columns?: DbColumn[]
  limits?: DbViewerLimits
}) {
  const { t } = useTranslation('databases')
  const copy = useCopy()
  const [offset, setOffset] = useState(0)
  const [sort, setSort] = useState<{ column: string; desc: boolean } | null>(
    null,
  )
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [applied, setApplied] = useState<DbColumnFilter[]>([])
  const [selected, setSelected] = useState<ReadonlySet<number>>(new Set())
  const [activeRow, setActiveRow] = useState(0)
  const [drawerRow, setDrawerRow] = useState<number | null>(null)

  useEffect(() => {
    const id = window.setTimeout(() => {
      const next = Object.entries(draft).flatMap(([c, v]) => {
        const f = parseFilterInput(c, v)
        return f ? [f] : []
      })
      setApplied((cur) =>
        JSON.stringify(cur) === JSON.stringify(next) ? cur : next,
      )
      setOffset(0)
    }, FILTER_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(id)
    }
  }, [draft])

  const params: DbPageParams = {
    schema,
    table,
    limit: PAGE_SIZE,
    offset,
    sort: sort?.column,
    desc: sort?.desc,
    filters: applied,
  }
  const { data, isLoading, error, isFetching } = useDatabaseTablePage(
    databaseName,
    params,
  )

  const gridColumns = useMemo((): GridColumn[] => {
    const byName = new Map((meta ?? []).map((c) => [c.name, c]))
    return (data?.columns ?? []).map((name) => {
      const m = byName.get(name)
      return {
        name,
        type: m?.type,
        kind: columnKind(m?.type ?? ''),
        primaryKey: m?.primary_key ?? false,
      }
    })
  }, [data?.columns, meta])

  const pageKey = JSON.stringify([offset, sort, applied])
  const [seenPageKey, setSeenPageKey] = useState(pageKey)
  if (seenPageKey !== pageKey) {
    setSeenPageKey(pageKey)
    setSelected(new Set())
    setActiveRow(0)
  }

  function toggleSort(column: string) {
    setOffset(0)
    setSort((cur) =>
      cur?.column === column
        ? { column, desc: !cur.desc }
        : { column, desc: false },
    )
  }

  function copyAs(format: 'csv' | 'json' | 'insert') {
    if (!data) return
    const idx =
      selected.size > 0
        ? [...selected].sort((a, b) => a - b)
        : data.rows.map((_, i) => i)
    const rows = idx.flatMap((i) => (data.rows[i] ? [data.rows[i]] : []))
    const kinds = gridColumns.map((c) => c.kind)
    const text =
      format === 'csv'
        ? toCsv(data.columns, rows)
        : format === 'json'
          ? toJson(data.columns, rows, kinds)
          : toInsert(
              dialectOf(engine),
              schema,
              table,
              data.columns,
              rows,
              kinds,
            )
    copy(text, `viewer.grid.copied_${format}`)
  }

  const rowCount = data?.row_count ?? 0
  const filtering = applied.length > 0

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {filtering
            ? t('viewer.data.filterActive', { count: applied.length })
            : t('viewer.data.filterHint')}
        </p>
        <div
          role="group"
          aria-label={t('viewer.data.copyAs')}
          className="flex items-center gap-1.5"
        >
          <span className="text-xs text-muted-foreground">
            {selected.size > 0
              ? t('viewer.data.copySelected', { count: selected.size })
              : t('viewer.data.copyAll', { count: rowCount })}
          </span>
          {(['csv', 'json', 'insert'] as const).map((f) => (
            <Button
              key={f}
              type="button"
              size="sm"
              variant="outline"
              disabled={!data || rowCount === 0}
              onClick={() => {
                copyAs(f)
              }}
            >
              <CopyIcon className="size-3.5" aria-hidden="true" />
              {t(`viewer.data.format.${f}`)}
            </Button>
          ))}
        </div>
      </div>

      {isLoading ? (
        <Skeleton className="h-48 w-full" />
      ) : error ? (
        <p className="text-sm text-destructive" role="alert">
          {error.message}
        </p>
      ) : data ? (
        <>
          {data.rows.length === 0 ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              {filtering || offset > 0
                ? t('viewer.data.noRows')
                : t('viewer.data.noRowsEmptyTable')}
            </p>
          ) : null}
          {data.rows.length > 0 || filtering ? (
            <DataGrid
              columns={gridColumns}
              rows={data.rows}
              sort={sort}
              onSort={toggleSort}
              filters={draft}
              onFilterChange={(column, value) => {
                setDraft((d) => ({ ...d, [column]: value }))
              }}
              selected={selected}
              onSelectedChange={setSelected}
              activeRow={activeRow}
              onActiveRowChange={setActiveRow}
              onOpenRow={setDrawerRow}
            />
          ) : null}
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
            <span aria-live="polite">
              {rowCount > 0
                ? t('viewer.data.showing', {
                    from: offset + 1,
                    to: offset + rowCount,
                    size: PAGE_SIZE,
                  })
                : ''}
              {sort
                ? ` | ${t('viewer.data.sortedBy', { column: sort.column })}`
                : ''}
              {data.truncated ? ` | ${t('viewer.grid.resultCapped')}` : ''}
              {isFetching ? ` | ${t('viewer.data.loading')}` : ''}
            </span>
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={offset === 0 || isFetching}
                onClick={() => {
                  setOffset(Math.max(0, offset - PAGE_SIZE))
                }}
              >
                {t('viewer.data.previous')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={!data.has_more || isFetching}
                onClick={() => {
                  setOffset(offset + PAGE_SIZE)
                }}
              >
                {t('viewer.data.next')}
              </Button>
            </div>
          </div>
          {limits ? (
            <p className="text-xs text-muted-foreground">
              {t('viewer.data.limitsNote', {
                page: PAGE_SIZE,
                rows: limits.max_rows,
                cell: formatSize(limits.max_cell_bytes),
                seconds: Math.round(limits.timeout_ms / 1000),
              })}
            </p>
          ) : null}
          <RowDetailDrawer
            open={drawerRow !== null}
            onOpenChange={(open) => {
              if (!open) setDrawerRow(null)
            }}
            title={`${schema}.${table}`}
            columns={gridColumns}
            rows={data.rows}
            rowIndex={drawerRow ?? 0}
            onRowChange={(i) => {
              setDrawerRow(i)
              setActiveRow(i)
            }}
            cellCapBytes={limits?.max_cell_bytes}
          />
        </>
      ) : null}
    </div>
  )
}
