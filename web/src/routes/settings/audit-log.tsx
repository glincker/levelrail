import { createFileRoute } from '@tanstack/react-router'
import { Input } from '@/components/ui/input'
import { useEffect, useRef, useState } from 'react'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  ClockCounterClockwiseIcon,
  DownloadSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '../../components/ui/table'
import { Badge } from '../../components/ui/badge'
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
  CLIENT_KIND_OPTIONS,
  type AuditLogEntry,
} from '../../queries/auditLog'
import { TableSkeleton } from '../../components/ui/table-skeleton'
import { PurgeAuditLogDialog } from '../../components/PurgeAuditLogDialog'

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

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Matches the badge convention every other status-flavored table in
// this app already uses (UserTable/TokenTable): success for a 2xx
// response, destructive for anything else, since a non-2xx from an
// AbilityWrite-or-higher route is exactly the kind of thing an operator
// scanning this table needs to notice first.
function StatusBadge({ status }: { status: number }) {
  const ok = status >= 200 && status < 300
  return <Badge variant={ok ? 'success' : 'destructive'}>{status}</Badge>
}

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
}: {
  clientKind?: string
  search?: string
  failedOnly?: boolean
}) {
  return (
    <a
      href={auditLogExportURL({ clientKind, search, failedOnly })}
      download
      className={buttonVariants({ variant: 'outline', size: 'sm' })}
    >
      <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
      Export CSV
    </a>
  )
}

// Client kind labels mirror internal/api's ClientKindCLI/Dashboard/MCP/API
// constants for display; CLIENT_KIND_OPTIONS (queries/auditLog.ts) is the
// source of truth for which values exist.
const CLIENT_KIND_LABELS: Record<string, string> = {
  cli: 'CLI',
  dashboard: 'Dashboard',
  mcp: 'MCP',
  api: 'API',
}

function ClientKindBadge({ clientKind }: { clientKind: string }) {
  return (
    <Badge variant="outline">
      {CLIENT_KIND_LABELS[clientKind] ?? clientKind}
    </Badge>
  )
}

function AuditLogSettingsPage() {
  const { data: initial } = useSuspenseQuery(auditLogQueryOptions())
  const [entries, setEntries] = useState<AuditLogEntry[]>(initial)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [clientKindFilter, setClientKindFilter] = useState(ALL_CLIENT_KINDS)
  const [filterLoading, setFilterLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [failedOnly, setFailedOnly] = useState(false)
  // A page shorter than the default limit means the store had no more
  // rows to return, the same "short page means done" signal offset-free
  // cursor pagination always relies on.
  const [exhausted, setExhausted] = useState(initial.length < AUDIT_PAGE_SIZE)

  const debouncedSearch = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS)

  const activeClientKind =
    clientKindFilter === ALL_CLIENT_KINDS ? undefined : clientKindFilter
  const activeSearch = debouncedSearch === '' ? undefined : debouncedSearch
  const filtersActive =
    activeClientKind !== undefined || activeSearch !== undefined || failedOnly

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
  }, [activeClientKind, activeSearch, failedOnly])

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
          <div>
            <h1 className="text-lg font-semibold text-foreground">Audit log</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Who changed what: every write, deploy, or root-tier request,
              newest first. Read-only requests aren't recorded here.
            </p>
          </div>
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
      </div>

      {filterLoading ? (
        <TableSkeleton columnCount={7} rowCount={8} />
      ) : entries.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border px-6 py-12 text-center">
          <div className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <ClockCounterClockwiseIcon className="size-5" />
          </div>
          <p className="text-sm text-muted-foreground">
            {filtersActive
              ? 'No entries match these filters.'
              : 'No audited requests recorded yet.'}
          </p>
        </div>
      ) : (
        <>
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Client</TableHead>
                  <TableHead>Ability</TableHead>
                  <TableHead>Method</TableHead>
                  <TableHead>Path</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((entry) => (
                  <TableRow key={entry.id}>
                    <TableCell className="text-muted-foreground">
                      {formatDate(entry.created_at)}
                    </TableCell>
                    <TableCell className="font-medium text-foreground">
                      {entry.actor_name}
                      <Badge variant="outline" className="ml-2">
                        {entry.actor_type}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <ClientKindBadge clientKind={entry.client_kind} />
                    </TableCell>
                    <TableCell>{entry.ability}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {entry.method}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {entry.path}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={entry.status_code} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      {loadMoreError ? (
        <p className="text-sm text-destructive">{loadMoreError}</p>
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
// page's own 6-column table shape so the skeleton doesn't jump when real
// rows swap in.
function AuditLogSettingsPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">Audit log</h1>
      <TableSkeleton columnCount={7} rowCount={8} />
    </div>
  )
}
