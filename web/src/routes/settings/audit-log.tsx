import { createFileRoute } from '@tanstack/react-router'
import { Input } from '@/components/ui/input'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import {
  DownloadGlassIcon,
  RestoreGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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
import { AuditIntegrityBadge } from '../../components/AuditIntegrityBadge'
import { AuditLogTable } from '../../components/AuditLogTable'
import { AgentFilterChips } from '../../components/AgentFilterChips'
import { collectAgentNames } from '../../lib/agentNames'
import { tokenListQueryOptions } from '../../queries/tokens'
import { domainsQueryOptions } from '../../queries/domains'
import { EmptyState } from '../../components/ui/empty-state'
import { PageHeader } from '@/components/shell/PageHeader'
import { PageTemplate } from '@/components/shell/PageTemplate'

// ALL_CLIENT_KINDS is the filter dropdown's "no filter" sentinel: Base
// UI's Select cannot use an empty string as an item value (it reads as
// "no selection"), the same reasoning DrainNodeDialog's own
// LOCAL_NODE_VALUE sentinel documents.
const ALL_CLIENT_KINDS = '__all__'

// The audit log's shared event family for CLI device logins.
const DEVICE_LOGIN_ACTION = 'device_login'

const AUDIT_PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300

// AbilityRoot-gated (GET /api/v1/audit-log): who changed what, across
// every session and API token. Same sensitivity tier as the node and
// GitHub App settings pages, not an ordinary read like Users.
export const Route = createFileRoute('/settings/audit-log')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { agent?: string; action?: string; path?: string } => ({
    ...(typeof search.agent === 'string' && search.agent !== ''
      ? { agent: search.agent }
      : {}),
    ...(typeof search.action === 'string' && search.action !== ''
      ? { action: search.action }
      : {}),
    ...(typeof search.path === 'string' && search.path !== ''
      ? { path: search.path }
      : {}),
  }),
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
  action,
  path,
}: {
  clientKind?: string
  search?: string
  failedOnly?: boolean
  agent?: string
  action?: string
  path?: string
}) {
  return (
    <a
      href={auditLogExportURL({
        clientKind,
        search,
        failedOnly,
        agent,
        action,
        path,
      })}
      download
      className={buttonVariants({ variant: 'outline', size: 'sm' })}
    >
      <DownloadGlassIcon className="size-5" aria-hidden="true" />
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
  const {
    agent: agentFromLink,
    action: actionFromLink,
    path: pathFromLink,
  } = Route.useSearch()
  const [agentFilter, setAgentFilter] = useState<string | undefined>(
    agentFromLink,
  )
  const [actionFilter, setActionFilter] = useState<string | undefined>(
    actionFromLink,
  )
  const [pathFilter, setPathFilter] = useState<string | undefined>(pathFromLink)
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
    agentFilter !== undefined ||
    actionFilter !== undefined ||
    pathFilter !== undefined

  // Server-driven: any filter change refetches the first page. The first
  // run is skipped because the route loader already supplied it, unless a
  // link opened the page already filtered.
  const firstRun = useRef(
    agentFromLink === undefined &&
      actionFromLink === undefined &&
      pathFromLink === undefined,
  )
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
      action: actionFilter,
      path: pathFilter,
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
  }, [
    activeClientKind,
    activeSearch,
    failedOnly,
    agentFilter,
    actionFilter,
    pathFilter,
  ])

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
        action: actionFilter,
        path: pathFilter,
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
    <PageTemplate
      icon={<RestoreGlassIcon size="1.4em" />}
      title={t('page.title')}
      description={t('page.description')}
      width="wide"
      actions={
        <>
          <Select
            value={clientKindFilter}
            onValueChange={(value) => {
              if (value) {
                setClientKindFilter(value)
              }
            }}
          >
            <SelectTrigger
              aria-label={t('filters.clientLabel')}
              className="w-36"
            >
              <SelectValue placeholder={t('filters.allClients')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL_CLIENT_KINDS}>
                {t('filters.allClients')}
              </SelectItem>
              {CLIENT_KIND_OPTIONS.map((kind) => (
                <SelectItem key={kind} value={kind}>
                  {CLIENT_KIND_LABELS[kind] ?? kind}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <AuditIntegrityBadge />
          <PurgeAuditLogDialog />
          {entries.length > 0 ? (
            <ExportAuditLogLink
              clientKind={activeClientKind}
              search={activeSearch}
              failedOnly={failedOnly}
              agent={agentFilter}
              action={actionFilter}
              path={pathFilter}
            />
          ) : null}
        </>
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
          }}
          placeholder={t('filters.searchPlaceholder')}
          aria-label={t('filters.searchLabel')}
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
          {t('filters.failedOnly')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant={actionFilter === DEVICE_LOGIN_ACTION ? 'default' : 'outline'}
          aria-pressed={actionFilter === DEVICE_LOGIN_ACTION}
          onClick={() => {
            setActionFilter((prev) =>
              prev === DEVICE_LOGIN_ACTION ? undefined : DEVICE_LOGIN_ACTION,
            )
          }}
        >
          {t('filters.deviceLogin')}
        </Button>
        {pathFilter ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setPathFilter(undefined)
            }}
          >
            {t('filters.clearPath')}
          </Button>
        ) : null}
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
          className="gap-2 border-0 bg-transparent py-4"
          icon={<RestoreGlassIcon className="size-6" />}
          title={t('page.emptyTitle')}
          description={
            filtersActive ? t('page.emptyFiltered') : t('page.emptyNone')
          }
        />
      ) : (
        <AuditLogTable entries={entries} domainApp={domainApp} />
      )}

      {loadMoreError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.4em" />
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
          {loadingMore ? t('filters.loading') : t('filters.loadOlder')}
        </Button>
      ) : null}
    </PageTemplate>
  )
}

// Route-level fallback for the loader's pending phase, matching this
// page's own 8-column table shape so the skeleton doesn't jump when real
// rows swap in.
function AuditLogSettingsPending() {
  const { t } = useTranslation('auditLog')
  return (
    <div className="space-y-6">
      <PageHeader title={t('page.title')} />
      <TableSkeleton columnCount={8} rowCount={8} />
    </div>
  )
}
