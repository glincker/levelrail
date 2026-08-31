import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  CloudArrowUpIcon,
  DownloadSimpleIcon,
  HardDrivesIcon,
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
  VOLUME_BACKUP_HISTORY_PAGE_SIZE,
  fetchVolumeBackupHistory,
  useTriggerVolumeBackup,
  useVolumeBackupHistory,
  volumeBackupDownloadURL,
} from '../queries/volumeBackupHistory'
import { useVolumeRestoreHistory } from '../queries/volumeRestoreHistory'
import { RestoreVolumeBackupDialog } from './RestoreVolumeBackupDialog'
import { VolumeBackupScheduleForm } from './VolumeBackupScheduleForm'
import { VolumeBackupVerificationBadge } from './VolumeBackupVerificationBadge'
import { StatusBadge } from './backupAttemptStatus'
import type { BackupHistoryRecord } from '../types/backupHistory'
import type { RestoreHistoryRecord } from '../types/restoreHistory'
import type { AppVolume } from '../types/appDetail'

// Trigger-and-history section for one app's named Docker volumes, the
// direct app-volume counterpart of BackupsSection (which does this for a
// managed database's own volume, implicitly, one per database). An app
// can declare any number of volumes in app.yaml, so this section adds a
// volume picker BackupsSection never needed, and every sub-component
// below takes an explicit volumeName instead of assuming there's only
// one.
//
// See queries/volumeBackupHistory.ts, queries/volumeRestoreHistory.ts,
// queries/volumeBackupVerification.ts, queries/volumeBackupSchedule.ts
// for the wire layer this composes, and BackupsSection's own header
// comment for the reasoning this mirrors in full (non-suspense queries,
// self-terminating polling while an attempt is running, the schedule
// form's daily/weekly/custom-cron shape via lib/cronSchedule.ts).

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

function TriggerVolumeBackupRow({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const targetsQuery = useBackupTargetsOptional()
  const targets = targetsQuery.data ?? []
  const [targetId, setTargetId] = useState<string>('')
  const triggerBackup = useTriggerVolumeBackup(appName, volumeName)

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
          <FieldLabel htmlFor="volume-backup-target-picker">
            Backup target
          </FieldLabel>
          <Select
            value={targetId}
            onValueChange={(value: string | null) => {
              setTargetId(value ?? '')
            }}
          >
            <SelectTrigger id="volume-backup-target-picker" className="w-full">
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

function DownloadVolumeBackupLink({
  appName,
  volumeName,
  backup,
}: {
  appName: string
  volumeName: string
  backup: BackupHistoryRecord
}) {
  return (
    <a
      href={volumeBackupDownloadURL(appName, volumeName, backup.id)}
      download
      className={buttonVariants({ variant: 'outline', size: 'sm' })}
    >
      <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
      Download
    </a>
  )
}

function VolumeBackupHistoryTable({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const targetsQuery = useBackupTargetsOptional()
  const { data, isLoading, error } = useVolumeBackupHistory(appName, volumeName)
  const firstPage = useMemo(() => data ?? [], [data])
  const [olderRows, setOlderRows] = useState<BackupHistoryRecord[]>([])
  const [loadingMore, setLoadingMore] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [noMoreOlder, setNoMoreOlder] = useState(false)
  const history = [...firstPage, ...olderRows]

  const exhausted =
    olderRows.length === 0
      ? firstPage.length < VOLUME_BACKUP_HISTORY_PAGE_SIZE
      : noMoreOlder

  async function handleLoadMore() {
    const oldest = history[history.length - 1]
    if (!oldest) return
    setLoadingMore(true)
    setLoadMoreError(null)
    try {
      const next = await fetchVolumeBackupHistory(appName, volumeName, {
        before: oldest.started_at,
      })
      setOlderRows((prev) => [...prev, ...next])
      if (next.length < VOLUME_BACKUP_HISTORY_PAGE_SIZE) {
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
    const targets = targetsQuery.data ?? []
    const byId = new Map(targets.map((t) => [t.id, t.name]))
    return (targetId: string) => byId.get(targetId) ?? 'Deleted target'
  }, [targetsQuery.data])

  if (isLoading) {
    return <TableSkeleton columnCount={7} rowCount={3} />
  }
  if (error) {
    return <p className="text-sm text-destructive">{error.message}</p>
  }
  if (history.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No backups triggered yet for this volume.
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
                    <VolumeBackupVerificationBadge
                      appName={appName}
                      volumeName={volumeName}
                      backup={record}
                    />
                  ) : (
                    <span className="text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell>
                  {record.status === 'succeeded' ? (
                    <div className="flex items-center gap-2">
                      <DownloadVolumeBackupLink
                        appName={appName}
                        volumeName={volumeName}
                        backup={record}
                      />
                      <RestoreVolumeBackupDialog
                        appName={appName}
                        volumeName={volumeName}
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

// VolumeRestoreHistoryTable mirrors RestoreHistoryTable's exact shape:
// rendered only once at least one restore has ever been triggered for
// this volume.
function VolumeRestoreHistoryTable({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const { data, isLoading, error } = useVolumeRestoreHistory(
    appName,
    volumeName,
  )
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

function NoVolumesDeclared() {
  return (
    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-border px-6 py-8 text-center">
      <HardDrivesIcon
        className="size-5 text-muted-foreground"
        aria-hidden="true"
      />
      <p className="text-sm text-muted-foreground">
        This app has no named volumes declared in app.yaml.
      </p>
    </div>
  )
}

export function AppVolumeBackupsSection({
  appName,
  volumes,
}: {
  appName: string
  volumes: AppVolume[] | undefined
}) {
  const [selected, setSelected] = useState<string | undefined>(
    volumes?.[0]?.name,
  )
  const volumeName = selected ?? volumes?.[0]?.name

  if (!volumes || volumes.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Volume backups</CardTitle>
        </CardHeader>
        <CardContent>
          <NoVolumesDeclared />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Volume backups</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {volumes.length > 1 ? (
          <Field className="w-full sm:w-64">
            <FieldLabel htmlFor="volume-picker">Volume</FieldLabel>
            <Select
              value={volumeName}
              onValueChange={(value: string | null) => {
                if (value) setSelected(value)
              }}
            >
              <SelectTrigger id="volume-picker" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {volumes.map((v) => (
                  <SelectItem key={v.name} value={v.name}>
                    {v.name} ({v.container_path})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
        {volumeName ? (
          <div className="space-y-4" key={volumeName}>
            <VolumeBackupScheduleForm
              appName={appName}
              volumeName={volumeName}
            />
            <TriggerVolumeBackupRow appName={appName} volumeName={volumeName} />
            <VolumeBackupHistoryTable
              appName={appName}
              volumeName={volumeName}
            />
            <VolumeRestoreHistoryTable
              appName={appName}
              volumeName={volumeName}
            />
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
