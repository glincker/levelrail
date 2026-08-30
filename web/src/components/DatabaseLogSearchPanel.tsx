import { useMemo, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { MagnifyingGlassIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { useDatabaseLogSearch } from '../queries/databaseLogs'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { LogSearchEmptyState } from './LogSearchEmptyState'
import {
  DEFAULT_TIME_RANGE_KEY,
  TIME_RANGE_PRESETS,
  resolveTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'

// Historical log search over GET /api/v1/databases/{name}/logs
// (internal/api/database_logs.go): the database-kind counterpart to
// LogSearchPanel.tsx. Same virtualized monospace row rendering and
// structured-field formatting, same time-range presets; only the query
// hook and endpoint differ.

const ROW_HEIGHT_PX = 22
const SEARCH_DEBOUNCE_MS = 300

export function DatabaseLogSearchPanel({
  databaseName,
}: {
  databaseName: string
}) {
  const [query, setQuery] = useState('')
  const debouncedQuery = useDebouncedValue(query, SEARCH_DEBOUNCE_MS)
  const [rangeKey, setRangeKey] = useState<TimeRangeKey>(DEFAULT_TIME_RANGE_KEY)
  const [refreshNonce, setRefreshNonce] = useState(0)

  const range = useMemo(
    () => resolveTimeRange(rangeKey),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, refreshNonce],
  )

  const { data, isLoading, error } = useDatabaseLogSearch(databaseName, {
    from: range.from,
    to: range.to,
    q: debouncedQuery.trim() || undefined,
  })
  const entries = data ?? []
  const search = { query: debouncedQuery, rangeKey, setQuery, setRangeKey }

  const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null)

  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 20,
  })

  return (
    <section id="log-search" className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <MagnifyingGlassIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Log search
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Searches stored container log entries in the selected range. Not a
            live tail; see the Live tab for that.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div
            role="group"
            aria-label="Time range"
            className="inline-flex rounded-md border border-border"
          >
            {TIME_RANGE_PRESETS.map((preset) => (
              <button
                key={preset.key}
                type="button"
                onClick={() => {
                  setRangeKey(preset.key)
                }}
                aria-pressed={rangeKey === preset.key}
                className={`px-2.5 py-1 text-xs font-medium first:rounded-l-md last:rounded-r-md ${
                  rangeKey === preset.key
                    ? 'bg-foreground text-background'
                    : 'bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
                }`}
              >
                {preset.label}
              </button>
            ))}
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setRefreshNonce((n) => n + 1)
            }}
          >
            Refresh
          </Button>
        </div>
      </div>

      <div className="relative mt-3">
        <MagnifyingGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          placeholder="Search log messages..."
          aria-label="Search log messages"
          className="pl-8 font-mono"
        />
        {query ? (
          <button
            type="button"
            onClick={() => {
              setQuery('')
            }}
            aria-label="Clear search"
            className="absolute top-1/2 right-2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
          >
            <XIcon className="size-3.5" aria-hidden="true" />
          </button>
        ) : null}
      </div>

      <div className="mt-3">
        {isLoading ? (
          <p className="px-1 py-2 text-sm text-muted-foreground">
            Searching...
          </p>
        ) : error ? (
          <p className="px-1 py-2 text-sm text-destructive">{error.message}</p>
        ) : entries.length === 0 ? (
          <LogSearchEmptyState resourceKind="database" search={search} />
        ) : (
          <>
            <p className="mb-1.5 px-1 text-xs text-muted-foreground">
              {entries.length.toLocaleString()}{' '}
              {entries.length === 1 ? 'entry' : 'entries'}
              {debouncedQuery ? ` matching "${debouncedQuery}"` : ''}
            </p>
            <div
              ref={setScrollEl}
              className="h-[50vh] overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 font-mono text-xs leading-5 text-neutral-200"
            >
              <div
                style={{
                  height: virtualizer.getTotalSize(),
                  position: 'relative',
                }}
              >
                {virtualizer.getVirtualItems().map((virtualRow) => {
                  const entry = entries[virtualRow.index]
                  if (!entry) {
                    return null
                  }
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
                        height: ROW_HEIGHT_PX,
                        transform: `translateY(${virtualRow.start}px)`,
                      }}
                      className="flex items-baseline gap-2 truncate px-3"
                    >
                      <span className="shrink-0 text-neutral-500">
                        {new Date(entry.timestamp).toLocaleTimeString()}
                      </span>
                      <span
                        className={`shrink-0 rounded px-1 text-[0.65rem] uppercase ${
                          entry.stream === 'stderr'
                            ? 'bg-red-900/40 text-red-300'
                            : 'bg-neutral-800 text-neutral-400'
                        }`}
                      >
                        {entry.stream}
                      </span>
                      {entry.structured ? (
                        <span className="shrink-0 rounded bg-sky-900/40 px-1 text-[0.65rem] uppercase text-sky-300">
                          json
                        </span>
                      ) : null}
                      <span
                        className={`truncate whitespace-pre ${
                          entry.stream === 'stderr'
                            ? 'text-red-400'
                            : 'text-neutral-200'
                        }`}
                      >
                        {entry.structured
                          ? formatStructuredFields(entry.fields, entry.message)
                          : entry.message}
                      </span>
                    </div>
                  )
                })}
              </div>
            </div>
          </>
        )}
      </div>
    </section>
  )
}

function formatStructuredFields(
  fields: Record<string, unknown> | undefined,
  fallbackMessage: string,
): string {
  if (!fields || Object.keys(fields).length === 0) {
    return fallbackMessage
  }
  return Object.entries(fields)
    .map(([key, value]) => `${key}=${stringifyFieldValue(value)}`)
    .join(' ')
}

function stringifyFieldValue(value: unknown): string {
  if (typeof value === 'string') {
    return value
  }
  return JSON.stringify(value)
}
