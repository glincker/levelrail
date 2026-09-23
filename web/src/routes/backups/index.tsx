import { createFileRoute, Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useSuspenseQuery } from '@tanstack/react-query'
import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import { formatBytes, formatDate } from '../../lib/format'
import { useBackupTargetsOptional } from '../../queries/backupTargets'
import {
  ALL_BACKUP_HISTORY_PAGE_SIZE,
  allBackupHistoryQueryOptions,
  fetchAllBackupHistory,
} from '../../queries/allBackupHistory'
import { StatusBadge } from '../../components/backupAttemptStatus'
import {
  AnyBackupDeleteDialog,
  AnyBackupDownloadLink,
  AnyBackupVerificationBadge,
} from '../../components/AnyBackupActions'
import type { BackupHistoryRecord } from '../../types/backupHistory'

// Instance-wide backup visibility: GET /api/v1/backups
// (internal/api/backups.go's own handleListAllBackups), merging every
// database's and every app volume's backup history into one newest-first
// list. Closes a real, currently-open gap: today a backup is only ever
// visible scoped to the one resource it belongs to (a database's own
// Overview page, an app's own Volumes page), so an operator with more
// than a couple of databases or apps has no single place to check "did
// everything back up last night." Directly validated by a competitor's
// own open feature request for the same thing (Coolify issue #7528,
// "Backup Manager in the UI", 47 comments).
//
// Top-level route, not nested under Settings: this is an operational
// view (what happened), not configuration. Cursor-paginated with a
// "Load older entries" button rather than virtualized, mirroring
// routes/settings/audit-log.tsx's own convention for the same kind of
// list, an unbounded, time-ordered attempt history, the same category as
// BackupsSection's own per-resource table (BackupsSection.tsx) and
// deploy history (routes/apps/$name/deploys/index.tsx): none of those
// virtualize either. This is a deliberate, judgment call for this list's
// specific access pattern, not a blanket exemption from the "lists over
// 50 items are virtualized" rule, which stays the right default for
// flat resource lists like Apps or Domains.
//
// Actions (download, verify, delete) reuse the exact mutations and
// dialogs already built for the per-resource pages
// (BackupVerificationBadge/VolumeBackupVerificationBadge,
// DeleteBackupDialog/DeleteVolumeBackupDialog, backupDownloadURL/
// volumeBackupDownloadURL) via AnyBackup*'s resource_kind dispatch
// (components/AnyBackupActions.tsx), not reimplemented here. Triggering a
// new backup and restoring are deliberately not offered from this page:
// both already require resource-specific context (which target, which
// confirmation flow) the per-resource pages own; this is a read-mostly
// visibility view, not a second place to configure or restore backups.
export const Route = createFileRoute('/backups/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(allBackupHistoryQueryOptions()),
  component: AllBackupsPage,
  pendingComponent: AllBackupsPending,
})

function resourceLabel(backup: BackupHistoryRecord): string {
  if (backup.resource_kind === 'volume') {
    return `${backup.service_name ?? ''}/${backup.volume_name ?? ''}`
  }
  return backup.database_name ?? ''
}

function ResourceLink({ backup }: { backup: BackupHistoryRecord }) {
  if (backup.resource_kind === 'volume') {
    return (
      <Link
        to="/apps/$name/volumes"
        params={{ name: backup.service_name ?? '' }}
        className="text-foreground underline-offset-2 hover:underline"
      >
        {resourceLabel(backup)}
      </Link>
    )
  }
  return (
    <Link
      to="/databases/$name/overview"
      params={{ name: backup.database_name ?? '' }}
      className="text-foreground underline-offset-2 hover:underline"
    >
      {resourceLabel(backup)}
    </Link>
  )
}

function KindBadge({ backup }: { backup: BackupHistoryRecord }) {
  return (
    <Badge variant="outline">
      {backup.resource_kind === 'volume' ? 'Volume' : 'Database'}
    </Badge>
  )
}

// Whether the empty-state's prerequisite branch ("add a backup target
// first") should show: only once the optional targets query has
// actually resolved, since it reports [] while still loading regardless
// of what's really configured, and that must not flash the prerequisite
// message ahead of a real answer.
export function shouldPromptForBackupTarget(
  targetsLoading: boolean,
  targetCount: number,
): boolean {
  return !targetsLoading && targetCount === 0
}

function AllBackupsPage() {
  const { data: initial } = useSuspenseQuery(allBackupHistoryQueryOptions())
  const [history, setHistory] = useState<BackupHistoryRecord[]>(initial)
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [exhausted, setExhausted] = useState(
    initial.length < ALL_BACKUP_HISTORY_PAGE_SIZE,
  )
  const targetsQuery = useBackupTargetsOptional()
  const targets = targetsQuery.data ?? []
  const noBackupTarget = shouldPromptForBackupTarget(
    targetsQuery.isLoading,
    targets.length,
  )
  const targetName = (targetId: string) =>
    targets.find((t) => t.id === targetId)?.name ?? 'Deleted target'

  async function handleLoadMore() {
    const oldest = history[history.length - 1]
    if (!oldest) return
    setLoadingMore(true)
    setLoadMoreError(null)
    try {
      const next = await fetchAllBackupHistory({ before: oldest.started_at })
      setHistory((prev) => [...prev, ...next])
      if (next.length < ALL_BACKUP_HISTORY_PAGE_SIZE) {
        setExhausted(true)
      }
    } catch (err) {
      setLoadMoreError(
        err instanceof Error ? err.message : 'failed to load older backups',
      )
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <CloudArrowUpIcon className="size-4" aria-hidden="true" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Backups</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Every backup attempt across every database and app volume, newest
            first. Manage a database's or app's own schedule and targets from
            its own page.
          </p>
        </div>
      </div>

      {history.length === 0 ? (
        noBackupTarget ? (
          <EmptyState
            icon={<CloudArrowUpIcon className="size-5" />}
            title="No backup target connected"
            description="Backups need somewhere to store to. Connect an S3-compatible bucket, then schedule or trigger a backup from a database's own page."
            action={
              <Button
                size="sm"
                render={<Link to="/settings/backup-targets" />}
                nativeButton={false}
              >
                Add a backup target
              </Button>
            }
          />
        ) : (
          <EmptyState
            icon={<CloudArrowUpIcon className="size-5" />}
            title="No backups yet"
            description="Backups will show up here once a schedule runs or you trigger one manually from a database's or app volume's own page."
          />
        )
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Kind</TableHead>
                <TableHead>Resource</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Started</TableHead>
                <TableHead>Finished</TableHead>
                <TableHead>Verification</TableHead>
                <TableHead>
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {history.map((backup) => (
                <TableRow key={backup.id}>
                  <TableCell>
                    <KindBadge backup={backup} />
                  </TableCell>
                  <TableCell>
                    <ResourceLink backup={backup} />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-1">
                      <StatusBadge status={backup.status} />
                      {backup.status === 'failed' && backup.error ? (
                        <span
                          className="max-w-[20rem] truncate text-xs text-destructive"
                          title={backup.error}
                        >
                          {backup.error}
                        </span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell className="text-foreground">
                    {targetName(backup.target_id)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {backup.status === 'running'
                      ? '-'
                      : formatBytes(backup.size_bytes)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(backup.started_at, '-')}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(backup.finished_at, '-')}
                  </TableCell>
                  <TableCell>
                    {backup.status === 'succeeded' ? (
                      <AnyBackupVerificationBadge backup={backup} />
                    ) : (
                      <span className="text-muted-foreground">-</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      {backup.status === 'succeeded' ? (
                        <AnyBackupDownloadLink backup={backup} />
                      ) : null}
                      {backup.status !== 'running' ? (
                        <AnyBackupDeleteDialog backup={backup} />
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {loadMoreError ? (
        <p className="text-sm text-destructive">{loadMoreError}</p>
      ) : null}

      {!exhausted && history.length > 0 ? (
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

function AllBackupsPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">Backups</h1>
      <TableSkeleton columnCount={9} rowCount={8} />
    </div>
  )
}
