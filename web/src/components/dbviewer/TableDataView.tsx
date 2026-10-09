import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useDatabaseTablePage } from '../../queries/databaseViewer'
import type {
  DbFilterOp,
  DbPageParams,
  DbTable,
} from '../../types/databaseViewer'
import { ResultGrid } from './ResultGrid'

const PAGE_SIZE = 100
const FILTER_OPS: DbFilterOp[] = ['contains', 'equals', 'is_null', 'not_null']
const NO_VALUE_OPS = new Set<DbFilterOp>(['is_null', 'not_null'])

interface Filter {
  column: string
  op: DbFilterOp
  value: string
}

// Paginated, sortable, filterable view of one table. Sorting and
// filtering are done by the database; column names are validated
// server-side against the table's own catalog.
export function TableDataView({
  databaseName,
  schema,
  table,
}: {
  databaseName: string
  schema: string
  table: DbTable
}) {
  const { t } = useTranslation('databases')
  const [offset, setOffset] = useState(0)
  const [sort, setSort] = useState<{ column: string; desc: boolean } | null>(
    null,
  )
  const [draft, setDraft] = useState<Filter>({
    column: table.columns[0]?.name ?? '',
    op: 'contains',
    value: '',
  })
  const [filter, setFilter] = useState<Filter | null>(null)

  const params: DbPageParams = {
    schema,
    table: table.name,
    limit: PAGE_SIZE,
    offset,
    sort: sort?.column,
    desc: sort?.desc,
    filterColumn: filter?.column,
    filterOp: filter?.op,
    filterValue: filter?.value,
  }
  const { data, isLoading, error, isFetching } = useDatabaseTablePage(
    databaseName,
    params,
  )

  function toggleSort(column: string) {
    setOffset(0)
    setSort((cur) =>
      cur?.column === column
        ? { column, desc: !cur.desc }
        : { column, desc: false },
    )
  }

  function applyFilter() {
    setOffset(0)
    setFilter(draft.column ? draft : null)
  }

  function clearFilter() {
    setOffset(0)
    setFilter(null)
    setDraft((d) => ({ ...d, value: '' }))
  }

  const rowCount = data?.row_count ?? 0

  return (
    <div className="space-y-3">
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          applyFilter()
        }}
      >
        <label className="space-y-1 text-xs text-muted-foreground">
          <span>{t('viewer.data.filterColumn')}</span>
          <select
            className="block h-8 rounded-lg border border-input bg-background px-2 text-sm text-foreground"
            value={draft.column}
            onChange={(e) => {
              setDraft({ ...draft, column: e.target.value })
            }}
          >
            {table.columns.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-1 text-xs text-muted-foreground">
          <span>{t('viewer.data.filterOp')}</span>
          <select
            className="block h-8 rounded-lg border border-input bg-background px-2 text-sm text-foreground"
            value={draft.op}
            onChange={(e) => {
              setDraft({ ...draft, op: e.target.value as DbFilterOp })
            }}
          >
            {FILTER_OPS.map((op) => (
              <option key={op} value={op}>
                {t(`viewer.data.ops.${op}`)}
              </option>
            ))}
          </select>
        </label>
        {NO_VALUE_OPS.has(draft.op) ? null : (
          <label className="min-w-40 flex-1 space-y-1 text-xs text-muted-foreground">
            <span>{t('viewer.data.filterValue')}</span>
            <Input
              value={draft.value}
              onChange={(e) => {
                setDraft({ ...draft, value: e.target.value })
              }}
            />
          </label>
        )}
        <Button type="submit" size="sm">
          {t('viewer.data.apply')}
        </Button>
        {filter ? (
          <Button type="button" size="sm" variant="ghost" onClick={clearFilter}>
            {t('viewer.data.clear')}
          </Button>
        ) : null}
      </form>

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
              {filter || offset > 0
                ? t('viewer.data.noRows')
                : t('viewer.data.noRowsEmptyTable')}
            </p>
          ) : (
            <ResultGrid
              columns={data.columns}
              rows={data.rows}
              sortColumn={sort?.column}
              sortDesc={sort?.desc}
              onSort={toggleSort}
            />
          )}
          <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
            <span>
              {rowCount > 0
                ? t('viewer.data.rowRange', {
                    from: offset + 1,
                    to: offset + rowCount,
                  })
                : ''}
              {sort
                ? ` | ${t('viewer.data.sortedBy', { column: sort.column })}`
                : ''}
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
        </>
      ) : null}
    </div>
  )
}
