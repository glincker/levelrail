import { createFileRoute } from '@tanstack/react-router'
import { Input } from '@/components/ui/input'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import {
  ClockCounterClockwiseIcon,
  DownloadSimpleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '../../components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../components/ui/select'
import {
  auditLogExportURL,
  auditLogQueryOptions,
  fetchAuditLog,
  CLIENT_KIND_LABELS,
  CLIENT_KIND_OPTIONS,
  type AuditLogEntry,
} from '../../queries/auditLog'
import { TableSkeleton } from '../../components/ui/table-skeleton'
import { PurgeAuditLogDialog } from '../../components/PurgeAuditLogDialog'
import { AuditLogTable } from '../../components/AuditLogTable'
import { AgentFilterChips } from '../../components/AgentFilterChips'
import { collectAgentNames } from '../../lib/agentNames'
import { tokenListQueryOptions } from '../../queries/tokens'
import { domainsQueryOptions } from '../../queries/domains'
import { EmptyState } from '../../components/ui/empty-state'
import { PageHeader } from '@/components/shell/PageHeader'

// ALL_CLIENT_KINDS is the filter dropdown's "no filter" sentinel: Base
// UI's Select cannot use an empty string as an item value (it reads as
// "no selection"), the same reasoning DrainNodeDialog's own
// LOCAL_NODE_VALUE sentinel documents.
const ALL_CLIENT_KINDS = '__all__'

const AUDIT_PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300

// AbilityRoot-gated (GET /api/v1/audit-log): who changed what, across
// every session and API token. Same sensitivity tier as the node and
// GitHub App settings pages, not an ordinary read like Users.
export const Route = createFileRoute('/settings/audit-log')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(auditLogQueryOptions()),
  component: AuditLogSettingsPage,
  pendingComponent: AuditLogSettingsPending,
})

// Downloads GET /api/v1/audit-log?format=csv as a plain browser
// navigation (<a href download>), the same pattern
// BackupsSection.tsx's DownloadBackupLink already establishes for a raw,
// non-JSON response: no fetch/blob dance needed since auth rides along
// on the same httpOnly session cookie every same-origin request uses.
// clientKind, when set, carries the live filter into the export so the
// downloaded CSV matches what's on screen.
function ExportAuditLogLink({
  clientKind,
  search,
  failedOnly,
  agent,
}: {
  clientKind?: string
  search?: string
  failedOnly?: boolean
  agent?: string
}) {
  return (
    <a
      href={auditLogExportURL({ clientKind, search, failedOnly, agent })}
      download
      className={buttonVariants({ variant: 'outline', size: 'sm' })}
    >
      <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
      Export CSV
    </a>
  )
}

function AuditLogSettingsPage() {
  const { t } = useTranslation('auditLog')
  const { data: initial } = useSuspenseQuery(auditLogQueryOptions())
  const [entries, setEntries] = useState<AuditLogEntry[]>(initial)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [clientKindFilter, setClientKindFilter] = useState(ALL_CLIENT_KINDS)
  const [filterLoading, setFilterLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [failedOnly, setFailedOnly] = useState(false)
  const [agentFilter, setAgentFilter] = useState<string | undefined>()
  const { data: tokens = [] } = useQuery({
    ...tokenListQueryOptions(),
    retry: false,
  })
  // Resolves a certificate audit row's domain tag to its owning app for
  // AuditLogTable's link; best-effort like tokens above, so a domains
  // fetch failure degrades to plain unlinked domain text, not a broken
  // page.
  const { data: domains = [] } = useQuery({
    ...domainsQueryOptions(),
    retry: false,
  })
  const domainApp = new Map(domains.map((d) => [d.domain, d.service_name]))
  // A page shorter than the default limit means the store had no more
  // rows to return, the same "short page means done" signal offset-free
  // cursor pagination always relies on.
  const [exhausted, setExhausted] = useState(initial.length < AUDIT_PAGE_SIZE)

  const debouncedSearch = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS)

  const activeClientKind =
    clientKindFilter === ALL_CLIENT_KINDS ? undefined : clientKindFilter
  const activeSearch = debouncedSearch === '' ? undefined : debouncedSearch
  const filtersActive =
    activeClientKind !== undefined ||
    activeSearch !== undefined ||
    failedOnly ||
    agentFilter !== undefined

  // Server-driven: any filter change refetches the first page. The first
  // run is skipped because the route loader already supplied it.
  const firstRun = useRef(true)
  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false
      return
    }
    let cancelled = false
    setLoadMoreError(null)
    setFilterLoading(true)
    fetchAuditLog({
      clientKind: activeClientKind,
      search: activeSearch,
      failedOnly,
      agent: agentFilter,
    })
      .then((next) => {
        if (cancelled) return
        setEntries(next)
        setExhausted(next.length < AUDIT_PAGE_SIZE)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setLoadMoreError(
          err instanceof Error ? err.message : 'failed to filter audit log',
        )
      })
      .finally(() => {
        if (!cancelled) setFilterLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [activeClientKind, activeSearch, failedOnly, agentFilter])

  async function handleLoadMore() {
    const last = entries[entries.length - 1]
    if (!last) return
    setLoadingMore(true)
    setLoadMoreError(null)
    try {
      const next = await fetchAuditLog({
        before: last.created_at,
        clientKind: activeClientKind,
        search: activeSearch,
        failedOnly,
        agent: agentFilter,
      })
      setEntries((prev) => [...prev, ...next])
      if (next.length < AUDIT_PAGE_SIZE) {
        setExhausted(true)
      }
    } catch (err) {
      setLoadMoreError(
        err instanceof Error ? err.message : 'failed to load more entries',
      )
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <ClockCounterClockwiseIcon className="size-4" />
          </div>
          <PageHeader title="Audit log" description={t('page.description')} />
        </div>
        <div className="flex items-center gap-2">
          <Select
            value={clientKindFilter}
            onValueChange={(value) => {
              if (value) {
                setClientKindFilter(value)
              }
            }}
          >
            <SelectTrigger aria-label="Filter by client" className="w-36">
              <SelectValue placeholder="All clients" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL_CLIENT_KINDS}>All clients</SelectItem>
              {CLIENT_KIND_OPTIONS.map((kind) => (
                <SelectItem key={kind} value={kind}>
                  {CLIENT_KIND_LABELS[kind] ?? kind}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <PurgeAuditLogDialog />
          {entries.length > 0 ? (
            <ExportAuditLogLink
              clientKind={activeClientKind}
              search={activeSearch}
              failedOnly={failedOnly}
              agent={agentFilter}
            />
          ) : null}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
          }}
          placeholder="Search actor, path, method, address"
          aria-label="Search audit log entries"
          className="max-w-xs"
        />
        <Button
          type="button"
          size="sm"
          variant={failedOnly ? 'default' : 'outline'}
          aria-pressed={failedOnly}
          onClick={() => {
            setFailedOnly((prev) => !prev)
          }}
        >
          Failed only
        </Button>
        <AgentFilterChips
          agents={collectAgentNames(tokens, entries, agentFilter)}
          active={agentFilter}
          onChange={setAgentFilter}
        />
      </div>

      {filterLoading ? (
        <TableSkeleton columnCount={8} rowCount={8} />
      ) : entries.length === 0 ? (
        <EmptyState
          icon={<ClockCounterClockwiseIcon className="size-5" />}
          title="No entries found"
          description={
            filtersActive
              ? 'No entries match these filters.'
              : 'No audited requests recorded yet.'
          }
        />
      ) : (
        <AuditLogTable entries={entries} domainApp={domainApp} />
      )}

      {loadMoreError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{loadMoreError}</AlertDescription>
        </Alert>
      ) : null}

      {!exhausted && entries.length > 0 ? (
        <Button
          type="button"
          variant="outline"
          disabled={loadingMore}
          onClick={() => {
            void handleLoadMore()
          }}
        >
          {loadingMore ? 'Loading...' : 'Load older entries'}
        </Button>
      ) : null}
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching this
// page's own 8-column table shape so the skeleton doesn't jump when real
// rows swap in.
function AuditLogSettingsPending() {
  return (
    <div className="space-y-6">
      <PageHeader title="Audit log" />
      <TableSkeleton columnCount={8} rowCount={8} />
    </div>
  )
}
