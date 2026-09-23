import { useMemo, useState } from 'react'
import {
  GaugeIcon,
  CaretUpIcon,
  CaretDownIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useDatabase } from '../queries/databases'
import { useDatabaseSlowQueries } from '../queries/databaseSlowQueries'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'
import type { SlowQueryEntry } from '../types/slowQueries'
import { Input } from './ui/input'
import { EmptyState } from './ui/empty-state'
import { Skeleton } from './ui/skeleton'
import { TimeRangeControls } from './TimeRangeControls'
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from './ui/table'

// Slow query log viewer for a Postgres/MySQL database, the slow-query
// counterpart to DatabaseLogSearchPanel: same time-range presets and
// refresh button, but a sortable/filterable table instead of a raw log
// line viewer, since GET /api/v1/databases/{name}/slow-queries
// (internal/api/database_slow_queries.go) already returns structured
// entries (timestamp, duration, query, rows examined) rather than plain
// text lines. Fetches a bounded page (MAX_FETCH_LIMIT) and sorts/filters
// it client-side: the server already caps how much history is parsed per
// request (internal/api/database_slow_queries.go's own limit/offset), so
// this stays a "one bounded page" view, not a paginated table.
//
// Engines other than Postgres/MySQL (Redis, MongoDB, MariaDB, KeyDB,
// Dragonfly, ClickHouse) have no slow-query-log support server-side yet
// (see internal/slowquery's package doc comment for why Redis in
// particular doesn't fit this log-parsing shape); this renders an
// explanatory empty state instead of firing a request that would 400.

const SUPPORTED_ENGINES = new Set(['postgres', 'mysql'])
const MAX_FETCH_LIMIT = 500
const MAX_QUERY_PREVIEW_CHARS = 300

type SortField = 'duration_ms' | 'timestamp'
type SortDir = 'asc' | 'desc'

function SlowQueriesSkeleton() {
  return (
    <div
      className="space-y-1.5 rounded-lg border border-border p-3"
      aria-hidden="true"
    >
      {['w-3/4', 'w-1/2', 'w-5/6', 'w-2/3', 'w-1/3'].map((width, i) => (
        <Skeleton key={i} className={`h-4 ${width}`} />
      ))}
    </div>
  )
}

export function DatabaseSlowQueriesPanel({
  databaseName,
}: {
  databaseName: string
}) {
  const { data: database } = useDatabase(databaseName)
  const engine = database?.engine ?? ''
  const supported = SUPPORTED_ENGINES.has(engine)

  const [rangeKey, setRangeKey] = useState<TimeRangeKey>(DEFAULT_TIME_RANGE_KEY)
  const [refreshNonce, setRefreshNonce] = useState(0)
  const [minDurationMs, setMinDurationMs] = useState('')
  const [sortField, setSortField] = useState<SortField>('duration_ms')
  const [sortDir, setSortDir] = useState<SortDir>('desc')

  const range = useMemo(
    () => resolveTimeRange(rangeKey),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, refreshNonce],
  )

  const { data, isLoading, error } = useDatabaseSlowQueries(databaseName, {
    from: range.from,
    to: range.to,
    limit: MAX_FETCH_LIMIT,
  })
  const total = data?.total ?? 0

  const minDurationNum =
    minDurationMs.trim() === '' ? undefined : Number(minDurationMs)
  const filtered = useMemo(() => {
    let out = data?.entries ?? []
    if (minDurationNum !== undefined && !Number.isNaN(minDurationNum)) {
      out = out.filter((e) => e.duration_ms >= minDurationNum)
    }
    return [...out].sort((a, b) => {
      const av =
        sortField === 'duration_ms' ? a.duration_ms : Date.parse(a.timestamp)
      const bv =
        sortField === 'duration_ms' ? b.duration_ms : Date.parse(b.timestamp)
      return sortDir === 'asc' ? av - bv : bv - av
    })
  }, [data, minDurationNum, sortField, sortDir])

  function toggleSort(field: SortField) {
    if (sortField === field) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortField(field)
      setSortDir('desc')
    }
  }

  return (
    <section id="slow-queries" className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <GaugeIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Slow queries
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Statements parsed from this database&apos;s own slow query log.
            Postgres and MySQL only.
          </p>
        </div>
        {supported ? (
          <TimeRangeControls
            rangeKey={rangeKey}
            onRangeChange={setRangeKey}
            onRefresh={() => {
              setRefreshNonce((n) => n + 1)
            }}
          />
        ) : null}
      </div>

      {!supported ? (
        <EmptyState
          className="mt-3"
          icon={<GaugeIcon className="size-5" />}
          title="Slow query log not available for this engine"
          description={
            engine
              ? `Slow query log parsing is only wired up for Postgres and MySQL databases; "${engine}" isn't supported. Redis-family engines expose SLOWLOG on the live server instead of a log file, a different data source this view doesn't read.`
              : 'Loading database details...'
          }
        />
      ) : (
        <>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <div className="relative w-40">
              <Input
                type="number"
                min={0}
                value={minDurationMs}
                onChange={(e) => {
                  setMinDurationMs(e.target.value)
                }}
                placeholder="Min duration (ms)"
                aria-label="Minimum duration in milliseconds"
              />
              {minDurationMs ? (
                <button
                  type="button"
                  onClick={() => {
                    setMinDurationMs('')
                  }}
                  aria-label="Clear minimum duration filter"
                  className="absolute top-1/2 right-2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <XIcon className="size-3.5" aria-hidden="true" />
                </button>
              ) : null}
            </div>
          </div>

          <div className="mt-3">
            {isLoading ? (
              <SlowQueriesSkeleton />
            ) : error ? (
              <p className="px-1 py-2 text-sm text-destructive">
                {error.message}
              </p>
            ) : filtered.length === 0 ? (
              <EmptyState
                icon={<GaugeIcon className="size-5" />}
                title="No slow queries in range"
                description="No statement in the selected time range crossed this engine's configured slow-query threshold."
              />
            ) : (
              <>
                <p className="mb-1.5 px-1 text-xs text-muted-foreground">
                  {filtered.length.toLocaleString()} of {total.toLocaleString()}{' '}
                  {total === 1 ? 'entry' : 'entries'} in range
                  {minDurationNum !== undefined
                    ? ` at or above ${minDurationNum}ms`
                    : ''}
                </p>
                <div className="max-h-[60vh] overflow-auto rounded-lg border border-border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>
                          <SortHeader
                            field="timestamp"
                            label="Timestamp"
                            sortField={sortField}
                            sortDir={sortDir}
                            onToggle={toggleSort}
                          />
                        </TableHead>
                        <TableHead>
                          <SortHeader
                            field="duration_ms"
                            label="Duration"
                            sortField={sortField}
                            sortDir={sortDir}
                            onToggle={toggleSort}
                          />
                        </TableHead>
                        <TableHead>Rows examined</TableHead>
                        <TableHead>Query</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filtered.map((entry, i) => (
                        <SlowQueryRow
                          key={`${entry.timestamp}-${i}`}
                          entry={entry}
                        />
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </>
            )}
          </div>
        </>
      )}
    </section>
  )
}

// SortHeader is declared at module scope, not inside
// DatabaseSlowQueriesPanel: a component defined inside a render function
// is re-created (and loses any state) on every re-render, which
// react-hooks/static-components flags. sortField/sortDir/onToggle come in
// as props instead of closing over DatabaseSlowQueriesPanel's own state.
function SortHeader({
  field,
  label,
  sortField,
  sortDir,
  onToggle,
}: {
  field: SortField
  label: string
  sortField: SortField
  sortDir: SortDir
  onToggle: (field: SortField) => void
}) {
  const active = sortField === field
  return (
    <button
      type="button"
      onClick={() => {
        onToggle(field)
      }}
      className="inline-flex items-center gap-1 font-medium hover:text-foreground"
      aria-sort={
        active ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'
      }
    >
      {label}
      {active ? (
        sortDir === 'asc' ? (
          <CaretUpIcon className="size-3" aria-hidden="true" />
        ) : (
          <CaretDownIcon className="size-3" aria-hidden="true" />
        )
      ) : null}
    </button>
  )
}

function SlowQueryRow({ entry }: { entry: SlowQueryEntry }) {
  const query =
    entry.query.length > MAX_QUERY_PREVIEW_CHARS
      ? entry.query.slice(0, MAX_QUERY_PREVIEW_CHARS) + '...'
      : entry.query
  return (
    <TableRow>
      <TableCell className="text-xs text-muted-foreground">
        {new Date(entry.timestamp).toLocaleString()}
      </TableCell>
      <TableCell>
        <span
          className={
            entry.duration_ms >= 1000
              ? 'font-medium text-destructive'
              : 'font-medium text-foreground'
          }
        >
          {entry.duration_ms.toLocaleString(undefined, {
            maximumFractionDigits: 2,
          })}{' '}
          ms
        </span>
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {entry.rows_examined !== undefined
          ? entry.rows_examined.toLocaleString()
          : '-'}
      </TableCell>
      <TableCell
        className="max-w-xl truncate font-mono text-xs whitespace-pre-wrap"
        title={entry.query}
      >
        {query}
      </TableCell>
    </TableRow>
  )
}
