import { useMemo, useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import type { UseMutationResult } from '@tanstack/react-query'
import {
  CloudArrowUpIcon,
  DownloadSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import { Skeleton } from '@/components/ui/skeleton'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { formatBytes, formatDate } from '../lib/format'
import { ApiError } from '../lib/apiError'
import { useBackupTargetsOptional } from '../queries/backupTargets'
import {
  BACKUP_HISTORY_PAGE_SIZE,
  backupDownloadURL,
  fetchBackupHistory,
  useBackupHistory,
  useTriggerBackup,
} from '../queries/backupHistory'
import { RestoreBackupDialog } from './RestoreBackupDialog'
import { RestoreHistoryTable } from './RestoreHistoryTable'
import { CloneRestoreDialog } from './CloneRestoreDialog'
import { CloneRestoreHistoryTable } from './CloneRestoreHistoryTable'
import { BackupScheduleForm } from './BackupScheduleForm'
import { BackupVerificationBadge } from './BackupVerificationBadge'
import { StatusBadge } from './backupAttemptStatus'
import type { BackupHistoryRecord } from '../types/backupHistory'
import type { DatabaseResource } from '../types/databaseDetail'

// Trigger-and-history section for one database's backups, wired against
// POST/GET /api/v1/databases/{name}/backups (internal/api/backups.go).
// Rendered from routes/databases/$name/overview.tsx, the database detail
// page's one section route today. The backup target picker reads
// useBackupTargetsOptional (queries/backupTargets.ts), the same
// non-suspending hook StorageAttachmentCard already uses on the app
// Overview route: this Card's target list is a secondary concern the
// overview page's own loader must not block its already-loaded
// database/conditions data on, so it degrades to an empty list while
// loading or on error rather than suspending the whole route.
//
// Backup history itself is a plain, non-suspense useQuery
// (useBackupHistory), the same "optional section, not something the
// loader needs warm before first paint" shape queries/alerts.ts's
// useAlertRules already establishes for AlertRulesPanel.
//
// Restore (POST/GET /api/v1/databases/{name}/restore(s),
// internal/api/restore.go) lives in this same Card: RestoreBackupDialog
// is triggered from a succeeded row in the backup history table below
// (restoring is always "from a specific past backup", so the action
// belongs on that row, not as a separate standalone control), and
// RestoreHistoryTable (its own file) shows past restore attempts the same
// self-terminating-polling way BackupHistoryTable already shows backup
// attempts. StatusBadge/formatDate/AttemptStatus live in
// backupAttemptStatus.tsx, shared between the two tables.
//
// CloneRestoreDialog/CloneRestoreHistoryTable (POST/GET .../restore-as-new,
// .../clone-restores, internal/api/database_clone_restore.go) sit right
// next to the in-place restore controls above: the non-destructive
// alternative, restoring a backup into a brand-new database instead of
// overwriting this one's own live data.

// Empty state shown in place of the picker/button when no backup target
// is connected yet: a target picker with nothing to pick from would just
// be a confusing disabled control, so this replaces it entirely with a
// direct link to where one gets created, the same "point at the real fix,
// don't leave a broken control on screen" reasoning
// CreateBackupTargetDialog's own 501 branch already follows for the
// master-key gap.
export function NoTargetsConfigured() {
  return (
    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-border px-6 py-8 text-center">
      <CloudArrowUpIcon
        className="size-5 text-muted-foreground"
        aria-hidden="true"
      />
      <p className="text-sm text-muted-foreground">
        No backup targets connected yet.
      </p>
      <Link
        to="/settings/backup-targets"
        className="text-sm text-primary underline underline-offset-2"
      >
        Connect a bucket in Settings
      </Link>
    </div>
  )
}

// TriggerBackupRowView is the shared target-picker-plus-button UI for
// starting a backup, used identically for a database's own backups and
// an app volume's backups: only the mutation (and its picker element id,
// so two of these can coexist on the volume-backups section without a
// duplicate-id a11y issue) differs between the two.
export function TriggerBackupRowView({
  pickerId,
  trigger,
}: {
  pickerId: string
  trigger: UseMutationResult<unknown, ApiError, string>
}) {
  const targetsQuery = useBackupTargetsOptional()
  const targets = targetsQuery.data ?? []
  const [targetId, setTargetId] = useState<string>('')

  if (targetsQuery.isLoading) {
    return <Skeleton className="h-16 w-full" />
  }

  if (targets.length === 0) {
    return <NoTargetsConfigured />
  }

  function handleTrigger() {
    if (!targetId) {
      return
    }
    trigger.mutate(targetId, {
      onSuccess: () => {
        toast.add({
          title: 'Backup started.',
          description: 'This list updates automatically once it finishes.',
          type: 'success',
        })
      },
      onError: (error: ApiError) => {
        toast.add({
          title: 'Could not start backup.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const notConfigured =
    trigger.isError &&
    trigger.error instanceof ApiError &&
    trigger.error.status === 501

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col items-start gap-3 sm:flex-row sm:items-end">
        <Field className="w-full sm:w-64">
          <FieldLabel htmlFor={pickerId}>Backup target</FieldLabel>
          <Select
            value={targetId}
            onValueChange={(value: string | null) => {
              setTargetId(value ?? '')
            }}
          >
            <SelectTrigger id={pickerId} className="w-full">
              <SelectValue placeholder="Choose a backup target..." />
            </SelectTrigger>
            <SelectContent>
              {targets.map((target) => (
                <SelectItem key={target.id} value={target.id}>
                  {target.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Button
          type="button"
          onClick={handleTrigger}
          disabled={!targetId || trigger.isPending}
        >
          <CloudArrowUpIcon aria-hidden="true" />
          {trigger.isPending ? 'Starting...' : 'Back up now'}
        </Button>
      </div>
      {notConfigured ? (
        <p className="text-sm text-destructive">{trigger.error.message}</p>
      ) : null}
    </div>
  )
}

function TriggerBackupRow({ databaseName }: { databaseName: string }) {
  const triggerBackup = useTriggerBackup(databaseName)
  return (
    <TriggerBackupRowView pickerId="backup-target-picker" trigger={triggerBackup} />
  )
}

// One succeeded backup's download action: GET
// .../backups/{historyId}/download (internal/api/backup_download.go)
// streams a raw file, not JSON, so this is a plain browser-navigated
// <a href download>, not a TanStack Query mutation the way
// RestoreBackupDialog's trigger is. Auth rides along on the same
// httpOnly session cookie every other same-origin request in this app
// already relies on (see backupDownloadURL's own doc comment), so no
// fetch/blob dance is needed here to attach a token.
// DownloadBackupLinkView is the shared browser-navigated download
// action, used identically for a database backup and a volume backup:
// only the URL builder differs.
export function DownloadBackupLinkView({ href }: { href: string }) {
  return (
    <a
      href={href}
      download
      className={buttonVariants({ variant: 'outline', size: 'sm' })}
    >
      <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
      Download
    </a>
  )
}

function DownloadBackupLink({
  databaseName,
  backup,
}: {
  databaseName: string
  backup: BackupHistoryRecord
}) {
  return (
    <DownloadBackupLinkView href={backupDownloadURL(databaseName, backup.id)} />
  )
}

// useBackupHistoryPagination is the shared "load older entries" state
// machine, plain component state rather than useInfiniteQuery, mirroring
// routes/settings/audit-log.tsx's pattern for its own cursor-paginated
// endpoint. Used identically for a database's own backups and an app
// volume's backups: only the fetch function and page size differ.
// eslint-disable-next-line react-refresh/only-export-components
export function useBackupHistoryPagination(
  firstPage: BackupHistoryRecord[],
  fetchMore: (before: string) => Promise<BackupHistoryRecord[]>,
  pageSize: number,
) {
  const [olderRows, setOlderRows] = useState<BackupHistoryRecord[]>([])
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [noMoreOlder, setNoMoreOlder] = useState(false)
  const history = [...firstPage, ...olderRows]

  // Before any "Load older" click, a first page shorter than the page
  // size already means there's nothing older; after one, noMoreOlder
  // (set by handleLoadMore below) is the only signal that matters.
  const exhausted =
    olderRows.length === 0 ? firstPage.length < pageSize : noMoreOlder

  async function handleLoadMore() {
    const oldest = history[history.length - 1]
    if (!oldest) return
    setLoadingMore(true)
    setLoadMoreError(null)
    try {
      const next = await fetchMore(oldest.started_at)
      setOlderRows((prev) => [...prev, ...next])
      if (next.length < pageSize) {
        setNoMoreOlder(true)
      }
    } catch (err) {
      setLoadMoreError(
        err instanceof Error ? err.message : 'failed to load older backups',
      )
    } finally {
      setLoadingMore(false)
    }
  }

  return { history, exhausted, loadingMore, loadMoreError, handleLoadMore }
}

// BackupHistoryTableView is the shared table shell for a database's own
// backups and an app volume's backups: identical columns, skeleton,
// empty, and load-more states. The two callers differ only in which
// per-row verification badge and action buttons they render and how
// target ids resolve to names, so those are render props / a lookup
// function rather than baked in here.
export function BackupHistoryTableView({
  isLoading,
  error,
  history,
  emptyMessage,
  exhausted,
  loadingMore,
  loadMoreError,
  onLoadMore,
  targetName,
  renderVerification,
  renderActions,
}: {
  isLoading: boolean
  error: Error | null
  history: BackupHistoryRecord[]
  emptyMessage: string
  exhausted: boolean
  loadingMore: boolean
  loadMoreError: string | null
  onLoadMore: () => void
  targetName: (targetId: string) => string
  renderVerification: (record: BackupHistoryRecord) => ReactNode
  renderActions: (record: BackupHistoryRecord) => ReactNode
}) {
  if (isLoading) {
    return <TableSkeleton columnCount={7} rowCount={3} />
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }
  if (history.length === 0) {
    return <p className="text-sm text-muted-foreground">{emptyMessage}</p>
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
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
            {history.map((record: BackupHistoryRecord) => (
              <TableRow key={record.id}>
                <TableCell>
                  <div className="flex flex-col gap-1">
                    <StatusBadge status={record.status} />
                    {record.status === 'failed' && record.error ? (
                      <span
                        className="max-w-[20rem] truncate text-xs text-destructive"
                        title={record.error}
                      >
                        {record.error}
                      </span>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell className="text-foreground">
                  {targetName(record.target_id)}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {record.status === 'running'
                    ? '-'
                    : formatBytes(record.size_bytes)}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(record.started_at, '-')}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(record.finished_at, '-')}
                </TableCell>
                <TableCell>
                  {record.status === 'succeeded' ? (
                    renderVerification(record)
                  ) : (
                    <span className="text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell>
                  {record.status === 'succeeded' ? (
                    <div className="flex items-center gap-2">
                      {renderActions(record)}
                    </div>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {loadMoreError ? (
        <p className="text-sm text-destructive">{loadMoreError}</p>
      ) : null}

      {!exhausted ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={loadingMore}
          onClick={onLoadMore}
        >
          {loadingMore ? 'Loading...' : 'Load older backups'}
        </Button>
      ) : null}
    </div>
  )
}

function BackupHistoryTable({ databaseName }: { databaseName: string }) {
  const targetsQuery = useBackupTargetsOptional()
  const { data, isLoading, error } = useBackupHistory(databaseName)
  const firstPage = useMemo(() => data ?? [], [data])
  const pagination = useBackupHistoryPagination(
    firstPage,
    (before) => fetchBackupHistory(databaseName, { before }),
    BACKUP_HISTORY_PAGE_SIZE,
  )

  const targetName = useMemo(() => {
    const targets = targetsQuery.data ?? []
    const byId = new Map(targets.map((t) => [t.id, t.name]))
    return (targetId: string) => byId.get(targetId) ?? 'Deleted target'
  }, [targetsQuery.data])

  return (
    <BackupHistoryTableView
      isLoading={isLoading}
      error={error}
      history={pagination.history}
      emptyMessage="No backups triggered yet for this database."
      exhausted={pagination.exhausted}
      loadingMore={pagination.loadingMore}
      loadMoreError={pagination.loadMoreError}
      onLoadMore={() => {
        void pagination.handleLoadMore()
      }}
      targetName={targetName}
      renderVerification={(record) => (
        <BackupVerificationBadge databaseName={databaseName} backup={record} />
      )}
      renderActions={(record) => (
        <>
          <DownloadBackupLink databaseName={databaseName} backup={record} />
          <RestoreBackupDialog databaseName={databaseName} backup={record} />
          <CloneRestoreDialog databaseName={databaseName} backup={record} />
        </>
      )}
    />
  )
}

export function BackupsSection({ database }: { database: DatabaseResource }) {
  const databaseName = database.name
  return (
    <Card>
      <CardHeader>
        <CardTitle>Backups</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <BackupScheduleForm database={database} />
        <TriggerBackupRow databaseName={databaseName} />
        <BackupHistoryTable databaseName={databaseName} />
        <RestoreHistoryTable databaseName={databaseName} />
        <CloneRestoreHistoryTable databaseName={databaseName} />
      </CardContent>
    </Card>
  )
}
