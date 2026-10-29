import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BellRingGlassIcon,
  CheckGlassIcon,
  DownloadGlassIcon,
  LinkGlassIcon,
  RestoreGlassIcon,
  SearchGlassIcon,
  SortAzGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { TextAlignLeftIcon } from '@phosphor-icons/react/dist/ssr'
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
import { LogSearchRows } from './LogSearchRows'
import { SavedLogQueriesMenu } from './SavedLogQueriesMenu'
import { CreateLogAlertDialog } from './CreateLogAlertDialog'
import { LogEntryDetail } from './LogEntryDetail'
import { useLogShortcuts } from '../hooks/useLogShortcuts'
import {
  rangeSeconds,
  rememberQuery,
  type RecentQuery,
} from '../lib/logExplorer'
import type { LogQuerySpec } from '../types/logExplorer'
import { LogLevelChips } from './LogLevelChips'
import {
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

// Relative range keys a saved query can reproduce.
const TIME_RANGE_KEYS: TimeRangeKey[] = ['15m', '1h', '6h', '24h', '7d', '30d']
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
  const [wrap, setWrap] = useState(false)
  const [selected, setSelected] = useState<number | null>(null)
  const [alertOpen, setAlertOpen] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)

  const trimmedQuery = debouncedQuery.trim()
  useEffect(() => {
    if (trimmedQuery !== (filters.q ?? '')) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
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
    signature: filters.signature,
    order: trimmedQuery ? filters.order : undefined,
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
  const current: RecentQuery = {
    q: trimmedQuery || undefined,
    level: filters.level,
    field: filters.field,
  }
  const currentSpec: LogQuerySpec = {
    q: current.q,
    level: filters.level,
    stream: filters.stream,
    fields: filters.field,
    container: filters.container,
    range: rangeKey,
  }
  useEffect(() => {
    if (data) {
      rememberQuery(appName, current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])
  const applySpec = (spec: LogQuerySpec) => {
    const secs = rangeSeconds(spec.range)
    const key = TIME_RANGE_KEYS.find((k) => rangeSeconds(k) === secs)
    if (key) {
      setRangeKeyState(key)
    }
    setQuery(spec.q ?? '')
    update({
      q: spec.q,
      level: spec.level,
      stream: spec.stream,
      container: spec.container,
      field: spec.fields?.length ? spec.fields : undefined,
      signature: undefined,
      from: spec.from,
      to: spec.to,
    })
  }
  const addFieldFilter = (path: string, value: string) => {
    const next = `${path}=${value}`
    if (!(filters.field ?? []).includes(next)) {
      update({ field: [...(filters.field ?? []), next] })
    }
  }
  useLogShortcuts({
    focusSearch: () => searchInputRef.current?.focus(),
    toggleWrap: () => {
      setWrap((w) => !w)
    },
    next: () => {
      setSelected((i) => Math.min(entries.length - 1, (i ?? -1) + 1))
    },
    previous: () => {
      setSelected((i) => Math.max(0, (i ?? 1) - 1))
    },
    close: () => {
      setSelected(null)
    },
  })

  const selectedEntry = selected === null ? undefined : entries[selected]

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
          <h2 className="flex items-center gap-1.5 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            <SearchGlassIcon
              className="size-4.5 text-muted-foreground"
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
          <SavedLogQueriesMenu
            appName={appName}
            current={current}
            currentSpec={currentSpec}
            onApply={applySpec}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setAlertOpen(true)
            }}
          >
            <BellRingGlassIcon className="size-4" aria-hidden="true" />
            {t('explorer.alertFromSearch')}
          </Button>
          <Button
            type="button"
            variant={wrap ? 'secondary' : 'outline'}
            size="sm"
            aria-pressed={wrap}
            onClick={() => {
              setWrap((w) => !w)
            }}
          >
            <TextAlignLeftIcon className="size-3.5" aria-hidden="true" />
            {t('explorer.wrap')}
          </Button>
          <a
            href={logDownloadURL(appName, queryState)}
            download
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            <DownloadGlassIcon className="size-4" aria-hidden="true" />
            Download logs
          </a>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={copyPermalink}
          >
            {copied ? (
              <CheckGlassIcon className="size-4" aria-hidden="true" />
            ) : (
              <LinkGlassIcon className="size-4" aria-hidden="true" />
            )}
            {copied ? t('logs.copied') : t('logs.copyPermalink')}
          </Button>
        </TimeRangeControls>
      </div>

      <div className="relative mt-3">
        <SearchGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          ref={searchInputRef}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          placeholder={t('explorer.searchPlaceholder')}
          aria-label={t('explorer.searchLabel')}
          aria-describedby="log-search-syntax"
          className="pl-8 font-mono"
        />
        {query ? (
          <button
            type="button"
            onClick={() => {
              setQuery('')
            }}
            aria-label={t('explorer.clearSearch')}
            className="absolute top-1/2 right-2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
          >
            <XGlassIcon className="size-4" aria-hidden="true" />
          </button>
        ) : null}
      </div>
      <div className="mt-1 flex flex-wrap items-center justify-between gap-2">
        <p id="log-search-syntax" className="text-xs text-muted-foreground">
          {t('explorer.searchSyntax')}
        </p>
        {trimmedQuery ? (
          <div
            role="group"
            aria-label={t('explorer.orderLabel')}
            className="flex items-center gap-1"
          >
            {(['newest', 'relevance'] as const).map((mode) => {
              const active = (filters.order ?? 'newest') === mode
              return (
                <Button
                  key={mode}
                  type="button"
                  size="sm"
                  variant={active ? 'secondary' : 'outline'}
                  aria-pressed={active}
                  onClick={() => {
                    update({ order: mode === 'relevance' ? mode : undefined })
                  }}
                >
                  {mode === 'relevance' ? (
                    <SortAzGlassIcon className="size-4" aria-hidden="true" />
                  ) : (
                    <RestoreGlassIcon className="size-4" aria-hidden="true" />
                  )}
                  {mode === 'relevance'
                    ? t('explorer.orderRelevance')
                    : t('explorer.orderNewest')}
                </Button>
              )
            })}
          </div>
        ) : null}
      </div>

      {filters.signature ? (
        <p className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
          {t('explorer.signatureFilter', { id: filters.signature })}
          <button
            type="button"
            className="underline"
            onClick={() => {
              update({ signature: undefined })
            }}
          >
            {t('explorer.clearSignature')}
          </button>
        </p>
      ) : null}

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
            <LogSearchRows
              entries={entries}
              showContainer={(data?.containers.length ?? 0) > 1}
              wrap={wrap}
              selected={selected}
              onSelect={setSelected}
            />
            {selectedEntry ? (
              <LogEntryDetail
                appName={appName}
                entry={selectedEntry}
                onAddFilter={addFieldFilter}
                onClose={() => {
                  setSelected(null)
                }}
              />
            ) : null}
          </>
        )}
      </div>
      {alertOpen ? (
        <CreateLogAlertDialog
          appName={appName}
          open={alertOpen}
          onOpenChange={setAlertOpen}
          defaults={{
            name: t('explorer.alertDefaultName', { app: appName }),
            query: current.q,
            level: filters.level,
            fields: filters.field,
          }}
        />
      ) : null}
    </section>
  )
}
