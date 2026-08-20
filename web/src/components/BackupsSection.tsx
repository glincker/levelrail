import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import type { VariantProps } from 'class-variance-authority'
import type { Icon } from '@phosphor-icons/react'
import {
  CheckCircleIcon,
  CloudArrowUpIcon,
  DownloadSimpleIcon,
  SpinnerIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
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
import { formatBytes } from '../lib/format'
import { ApiError } from '../lib/apiError'
import { useBackupTargets } from '../queries/backupTargets'
import {
  BACKUP_HISTORY_PAGE_SIZE,
  backupDownloadURL,
  fetchBackupHistory,
  useBackupHistory,
  useTriggerBackup,
} from '../queries/backupHistory'
import { useRestoreHistory } from '../queries/restoreHistory'
import { RestoreBackupDialog } from './RestoreBackupDialog'
import type { BackupHistoryRecord, BackupStatus } from '../types/backupHistory'
import type {
  RestoreHistoryRecord,
  RestoreStatus,
} from '../types/restoreHistory'

// Trigger-and-history section for one database's backups, wired against
// POST/GET /api/v1/databases/{name}/backups (internal/api/backups.go).
// Rendered from routes/databases/$name/overview.tsx, the database detail
// page's one section route today. The backup target picker reads
// useBackupTargets (queries/backupTargets.ts), the same suspense query
// routes/settings/backup-targets.tsx already primes; overview.tsx's own
// loader primes it again here so this Card never suspends past a route
// boundary with nothing to catch it, the same reasoning that route's own
// Promise.all already applies to the database detail/status pair.
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
// RestoreHistoryTable shows past restore attempts the same
// self-terminating-polling way BackupHistoryTable already shows backup
// attempts, via useRestoreHistory.

// Shared between the backup and restore history tables below: both
// status unions (BackupStatus, RestoreStatus) are the identical three
// literal strings, running/succeeded/failed, one attempt lifecycle
// reused for two different resources (see store.RestoreHistory's own
// Go-side doc comment for why the backend reuses the same status
// constants rather than defining a second, identically-valued set), so
// one badge/label/icon mapping and one StatusBadge component below
// covers both tables rather than duplicating all three per table.
type AttemptStatus = BackupStatus | RestoreStatus

const STATUS_LABEL: Record<AttemptStatus, string> = {
  running: 'Running',
  succeeded: 'Succeeded',
  failed: 'Failed',
}

const STATUS_BADGE_VARIANT: Record<
  AttemptStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  running: 'muted',
  succeeded: 'success',
  failed: 'destructive',
}

const STATUS_ICON: Record<AttemptStatus, Icon> = {
  running: SpinnerIcon,
  succeeded: CheckCircleIcon,
  failed: WarningCircleIcon,
}

function StatusBadge({ status }: { status: AttemptStatus }) {
  const StatusIcon = STATUS_ICON[status]
  return (
    <Badge variant={STATUS_BADGE_VARIANT[status]} className="rounded-full">
      <StatusIcon
        className={status === 'running' ? 'size-3 animate-spin' : 'size-3'}
        aria-hidden="true"
      />
      {STATUS_LABEL[status]}
    </Badge>
  )
}

function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

// Empty state shown in place of the picker/button when no backup target
// is connected yet: a target picker with nothing to pick from would just
// be a confusing disabled control, so this replaces it entirely with a
// direct link to where one gets created, the same "point at the real fix,
// don't leave a broken control on screen" reasoning
// CreateBackupTargetDialog's own 501 branch already follows for the
// master-key gap.
function NoTargetsConfigured() {
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

function TriggerBackupRow({ databaseName }: { databaseName: string }) {
  const { data: targets } = useBackupTargets()
  const [targetId, setTargetId] = useState<string>('')
  const triggerBackup = useTriggerBackup(databaseName)

  if (targets.length === 0) {
    return <NoTargetsConfigured />
  }

  function handleTrigger() {
    if (!targetId) {
      return
    }
    triggerBackup.mutate(targetId, {
      onSuccess: () => {
        toast.add({
          title: 'Backup started.',
          description: 'This list updates automatically once it finishes.',
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not start backup.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const notConfigured =
    triggerBackup.isError &&
    triggerBackup.error instanceof ApiError &&
    triggerBackup.error.status === 501

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col items-start gap-3 sm:flex-row sm:items-end">
        <Field className="w-full sm:w-64">
          <FieldLabel htmlFor="backup-target-picker">Backup target</FieldLabel>
          <Select
            value={targetId}
            onValueChange={(value: string | null) => {
              setTargetId(value ?? '')
            }}
          >
            <SelectTrigger id="backup-target-picker" className="w-full">
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
          disabled={!targetId || triggerBackup.isPending}
        >
          <CloudArrowUpIcon aria-hidden="true" />
          {triggerBackup.isPending ? 'Starting...' : 'Back up now'}
        </Button>
      </div>
      {notConfigured ? (
        <p className="text-sm text-destructive">
          {triggerBackup.error.message}
        </p>
      ) : null}
    </div>
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
function DownloadBackupLink({
  databaseName,
  backup,
}: {
  databaseName: string
  backup: BackupHistoryRecord
}) {
  return (
    <a
      href={backupDownloadURL(databaseName, backup.id)}
      download
      className={buttonVariants({ variant: 'outline', size: 'sm' })}
    >
      <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
      Download
    </a>
  )
}

// "Load older entries" via plain component state rather than
// useInfiniteQuery, mirroring routes/settings/audit-log.tsx's pattern
// for its own cursor-paginated endpoint.
function BackupHistoryTable({ databaseName }: { databaseName: string }) {
  const { data: targets } = useBackupTargets()
  const { data, isLoading, error } = useBackupHistory(databaseName)
  const firstPage = useMemo(() => data ?? [], [data])
  const [olderRows, setOlderRows] = useState<BackupHistoryRecord[]>([])
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [noMoreOlder, setNoMoreOlder] = useState(false)
  const history = [...firstPage, ...olderRows]

  // Before any "Load older" click, a first page shorter than the page
  // size already means there's nothing older; after one, noMoreOlder
  // (set by handleLoadMore below) is the only signal that matters.
  const exhausted =
    olderRows.length === 0
      ? firstPage.length < BACKUP_HISTORY_PAGE_SIZE
      : noMoreOlder

  async function handleLoadMore() {
    const oldest = history[history.length - 1]
    if (!oldest) return
    setLoadingMore(true)
    setLoadMoreError(null)
    try {
      const next = await fetchBackupHistory(databaseName, {
        before: oldest.started_at,
      })
      setOlderRows((prev) => [...prev, ...next])
      if (next.length < BACKUP_HISTORY_PAGE_SIZE) {
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

  const targetName = useMemo(() => {
    const byId = new Map(targets.map((t) => [t.id, t.name]))
    return (targetId: string) => byId.get(targetId) ?? 'Deleted target'
  }, [targets])

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading...</p>
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }
  if (history.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No backups triggered yet for this database.
      </p>
    )
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
                    <div className="flex items-center gap-2">
                      <DownloadBackupLink
                        databaseName={databaseName}
                        backup={record}
                      />
                      <RestoreBackupDialog
                        databaseName={databaseName}
                        backup={record}
                      />
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
          onClick={() => {
            void handleLoadMore()
          }}
        >
          {loadingMore ? 'Loading...' : 'Load older backups'}
        </Button>
      ) : null}
    </div>
  )
}

// RestoreHistoryTable is BackupHistoryTable's restore-direction
// counterpart: same shape (a status-first table, newest attempt on top,
// polling handled entirely by useRestoreHistory the same way
// useBackupHistory already drives BackupHistoryTable), rendered only once
// at least one restore has ever been triggered, so a database nobody has
// ever restored shows nothing extra here rather than an empty table with
// nothing in it.
function RestoreHistoryTable({ databaseName }: { databaseName: string }) {
  const { data, isLoading, error } = useRestoreHistory(databaseName)
  const history = data ?? []

  if (isLoading || history.length === 0) {
    return null
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-medium text-foreground">Restores</h3>
      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Status</TableHead>
              <TableHead>From backup</TableHead>
              <TableHead>Started</TableHead>
              <TableHead>Finished</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {history.map((record: RestoreHistoryRecord) => (
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
                <TableCell className="text-muted-foreground">
                  {record.backup_history_id}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(record.started_at, '-')}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(record.finished_at, '-')}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

export function BackupsSection({ databaseName }: { databaseName: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Backups</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <TriggerBackupRow databaseName={databaseName} />
        <BackupHistoryTable databaseName={databaseName} />
        <RestoreHistoryTable databaseName={databaseName} />
      </CardContent>
    </Card>
  )
}
