import { useEffect, useMemo, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  DownloadSimpleIcon,
  LinkIcon,
  MagnifyingGlassIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { LogFilterBar } from './LogFilterBar'
import { Button } from './ui/button'
import {
  buildLogQuery,
  formatFieldFilter,
  parseFieldFilters,
} from '../lib/logFilters'
import type { LogsSearch } from '../lib/observabilitySearch'
import { logDownloadURL, useLogSearch } from '../queries/logs'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { buttonVariants } from './ui/button'
import { Input } from './ui/input'
import { LogSearchEmptyState } from './LogSearchEmptyState'
import { LogLevelChips, LogLevelGutter } from './LogLevelChips'
import {
  bucketOf,
  countLevels,
  filterByLevels,
  summarizeShown,
  toggleLevel,
  type LevelBucket,
} from '../lib/logLevels'
import { Skeleton } from './ui/skeleton'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'
import { TimeRangeControls } from './TimeRangeControls'

// Historical log search over GET /api/v1/apps/{name}/logs
// (internal/api/logs.go): a full-text query over entries
// telemetry has already stored, distinct from the live SSE tail this
// panel now sits alongside as the "Search" tab in
// routes/apps/$name/logs.tsx (components/LiveLogViewer.tsx renders the
// "Live" tab, following the running container's own output as it
// streams). This panel searches a time range of already-persisted log
// entries after the fact, answering "why was this app slow at 3am last
// Tuesday" without leaving the dashboard, so it is a request/response
// query through TanStack Query, not a kept-open EventSource. It shares
// LiveLogViewer's visual language (monospace rows, stderr highlighted,
// TanStack Virtual) because that's still the right look for a log line,
// not because it shares any code path with it.

const ROW_HEIGHT_PX = 22
const SEARCH_DEBOUNCE_MS = 300
// Newest entries fetched per search; the response total says how many matched.
const LOG_SEARCH_LIMIT = 1000

// Log-line-shaped placeholder for the search's own in-flight window,
// varying bar widths so it reads as text rather than a generic block,
// matching the dark monospace panel the real rows render into below.
const SKELETON_ROW_WIDTHS = ['w-3/4', 'w-1/2', 'w-5/6', 'w-2/3', 'w-1/3']

function LogSearchSkeleton() {
  return (
    <div
      className="space-y-1.5 rounded-lg border border-neutral-800 bg-neutral-950 p-3"
      aria-hidden="true"
    >
      {SKELETON_ROW_WIDTHS.map((width, i) => (
        <Skeleton key={i} className={`h-3 ${width} bg-neutral-800`} />
      ))}
    </div>
  )
}

// With `onSearchChange` the filters live in the URL (route search params);
// without it the panel keeps them in local state.
export function LogSearchPanel({
  appName,
  search: searchProp,
  onSearchChange,
}: {
  appName: string
  search?: LogsSearch
  onSearchChange?: (next: LogsSearch) => void
}) {
  const { t } = useTranslation('observability')
  const [localSearch, setLocalSearch] = useState<LogsSearch>({})
  const filters = onSearchChange ? (searchProp ?? {}) : localSearch
  const update = (patch: Partial<LogsSearch>) => {
    const next = { ...filters, ...patch }
    if (onSearchChange) {
      onSearchChange(next)
    } else {
      setLocalSearch(next)
    }
  }

  const [query, setQuery] = useState(filters.q ?? '')
  const debouncedQuery = useDebouncedValue(query, SEARCH_DEBOUNCE_MS)
  const [rangeKey, setRangeKeyState] = useState<TimeRangeKey>(
    DEFAULT_TIME_RANGE_KEY,
  )
  const [refreshNonce, setRefreshNonce] = useState(0)
  const [levels, setLevels] = useState<ReadonlySet<LevelBucket>>(new Set())
  const [copied, setCopied] = useState(false)

  const trimmedQuery = debouncedQuery.trim()
  useEffect(() => {
    if (trimmedQuery !== (filters.q ?? '')) {
      update({ q: trimmedQuery || undefined })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trimmedQuery])

  const fixedFrom = filters.from
  const fixedTo = filters.to
  const setRangeKey = (key: TimeRangeKey) => {
    setRangeKeyState(key)
    if (fixedFrom || fixedTo) {
      update({ from: undefined, to: undefined })
    }
  }

  const range = useMemo(
    () =>
      fixedFrom && fixedTo
        ? { from: new Date(fixedFrom), to: new Date(fixedTo) }
        : resolveTimeRange(rangeKey),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, refreshNonce, fixedFrom, fixedTo],
  )
  const fieldFilters = useMemo(
    () => parseFieldFilters(filters.field ?? []),
    [filters.field],
  )
  const queryState = {
    from: range.from,
    to: range.to,
    q: trimmedQuery || undefined,
    level: filters.level,
    container: filters.container,
    stream: filters.stream,
    fields: fieldFilters,
  }

  const { data, isLoading, error } = useLogSearch(appName, {
    ...queryState,
    limit: LOG_SEARCH_LIMIT,
  })

  const copyPermalink = () => {
    const params = buildLogQuery({
      ...queryState,
      from: range.from,
      to: range.to,
    })
    params.set('tab', 'search')
    const url = `${window.location.origin}${window.location.pathname}?${params.toString()}`
    void navigator.clipboard.writeText(url).then(() => {
      setCopied(true)
      window.setTimeout(() => {
        setCopied(false)
      }, 2000)
    })
  }
  const loaded = useMemo(() => data?.entries ?? [], [data])
  const counts = useMemo(() => countLevels(loaded), [loaded])
  const entries = useMemo(
    () => filterByLevels(loaded, levels),
    [loaded, levels],
  )
  const search = { query: debouncedQuery, rangeKey, setQuery, setRangeKey }

  // A state-backed callback ref (not useRef): useVirtualizer needs a
  // re-render once the scroll container actually mounts so
  // getScrollElement has something to measure, which a plain ref alone
  // does not trigger.
  const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null)

  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 20,
  })

  return (
    // id is a scroll-to-anchor target for AlertRulesPanel's "View logs"
    // link on a crashlooping rule: the last-200-lines-that-fired payload
    // only ever reaches the configured notify_url (see AlertRulesPanel's
    // header comment), so the honest thing this dashboard can offer
    // instead is jumping straight to this app's own live/historical log
    // view, not fabricating a view of lines this API cannot return.
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
        <TimeRangeControls
          rangeKey={rangeKey}
          onRangeChange={setRangeKey}
          onRefresh={() => {
            setRefreshNonce((n) => n + 1)
          }}
        >
          <a
            href={logDownloadURL(appName, queryState)}
            download
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
            Download logs
          </a>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={copyPermalink}
          >
            {copied ? (
              <CheckIcon className="size-3.5" aria-hidden="true" />
            ) : (
              <LinkIcon className="size-3.5" aria-hidden="true" />
            )}
            {copied ? t('logs.copied') : t('logs.copyPermalink')}
          </Button>
        </TimeRangeControls>
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

      <LogFilterBar
        containers={data?.containers ?? []}
        container={filters.container}
        stream={filters.stream}
        level={filters.level}
        fields={fieldFilters}
        onContainer={(container) => {
          update({ container })
        }}
        onStream={(stream) => {
          update({ stream })
        }}
        onLevel={(level) => {
          update({ level })
        }}
        onFields={(next) => {
          update({
            field: next.length > 0 ? next.map(formatFieldFilter) : undefined,
          })
        }}
      />

      <div className="mt-3">
        {isLoading ? (
          <LogSearchSkeleton />
        ) : error ? (
          <p className="px-1 py-2 text-sm text-destructive">{error.message}</p>
        ) : loaded.length === 0 ? (
          <LogSearchEmptyState resourceKind="app" search={search} />
        ) : (
          <>
            <div className="mb-2 space-y-1.5">
              <LogLevelChips
                counts={counts}
                selected={levels}
                onToggle={(bucket) => {
                  setLevels((prev) => toggleLevel(prev, bucket))
                }}
              />
              <p className="px-1 text-xs text-muted-foreground">
                {summarizeShown(
                  entries.length,
                  loaded.length,
                  data?.total ?? loaded.length,
                  levels.size > 0,
                )}
                {debouncedQuery ? ` matching "${debouncedQuery}"` : ''}
              </p>
            </div>
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
                      className="flex items-baseline gap-2 truncate pr-3"
                    >
                      <LogLevelGutter
                        level={entry.level}
                        bucket={bucketOf(entry.level)}
                      />
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
                      {entry.container && (data?.containers.length ?? 0) > 1 ? (
                        <span className="shrink-0 font-mono text-[0.65rem] text-neutral-500">
                          {entry.container.slice(0, 8)}
                        </span>
                      ) : null}
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

// Structured entries render as compact `key=value` pairs instead of the
// raw JSON message, keeping the monospace row style's fixed line height
// (a pretty-printed multi-line JSON block would break the virtualizer's
// fixed-size row assumption, same estimateSize/ROW_HEIGHT_PX approach
// the live build-log viewer uses). Falls back to the raw message if
// `fields` came back empty for some reason (logs.go's own contract
// allows Structured=true with FieldsJSON omitted, see toLogEntryResource).
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
