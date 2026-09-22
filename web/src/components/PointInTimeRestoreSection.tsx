import { useMemo, useState } from 'react'
import {
  ClockCounterClockwiseIcon,
  InfoIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { formatDate } from '../lib/format'
import { ApiError } from '../lib/apiError'
import { StatusBadge } from './backupAttemptStatus'
import { TriggerBackupRowView } from './BackupsSection'
import {
  useBaseBackupHistory,
  useDisablePITR,
  useEnablePITR,
  usePITRRestoreHistory,
  usePITRStatus,
  useTriggerBaseBackup,
  useTriggerPITRRestore,
} from '../queries/pitr'
import type { DatabaseResource } from '../types/databaseDetail'
import type { BaseBackupHistoryRecord } from '../types/pitr'

// Point-in-time restore (internal/api/pitr.go), Postgres only today
// (WAL archiving is the only mechanism internal/reconcile/database's
// controller implements PITR through, and only for that engine): a
// separate card from BackupsSection rather than folded into it, since
// the two restore modes have genuinely different inputs (a specific past
// backup vs. an arbitrary timestamp within a continuously recoverable
// window) and BackupsSection is already near this codebase's own
// 500-line file budget.
//
// IMPORTANT CAVEAT, surfaced directly in this card whenever PITR isn't
// enabled yet: point-in-time restore only ever covers the time range
// from when it was turned on, forward. Nothing before that moment is
// recoverable by timestamp, only by an ordinary backup if one happened
// to exist from that period (BackupsSection's own restore flow).
export function PointInTimeRestoreSection({
  database,
}: {
  database: DatabaseResource
}) {
  if (database.engine !== 'postgres') {
    return null
  }
  return <PointInTimeRestoreCard databaseName={database.name} />
}

function PointInTimeRestoreCard({ databaseName }: { databaseName: string }) {
  const status = usePITRStatus(databaseName)
  const enablePITR = useEnablePITR(databaseName)
  const disablePITR = useDisablePITR(databaseName)

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Point-in-time restore</CardTitle>
        {status.data?.enabled ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disablePITR.isPending}
            onClick={() => {
              disablePITR.mutate(undefined, {
                onSuccess: () => {
                  toast.add({
                    title: 'Point-in-time restore disabled.',
                    description:
                      'Existing base backups and archived WAL are kept; only new archiving stops.',
                    type: 'success',
                  })
                },
              })
            }}
          >
            {disablePITR.isPending ? 'Disabling...' : 'Disable'}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {status.isLoading ? <Skeleton className="h-16 w-full" /> : null}

        {!status.isLoading && !status.data?.enabled ? (
          <div className="flex flex-col gap-3 rounded-lg border border-dashed border-border px-4 py-4">
            <p className="text-sm text-muted-foreground">
              Enable continuous WAL archiving so this database can be restored
              to any exact moment, not just to when a backup happened to run.
            </p>
            <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
              <InfoIcon
                className="mt-0.5 size-3.5 shrink-0"
                aria-hidden="true"
              />
              Only recoverable going forward from the moment you enable this.
              Nothing from before enabling it can be restored by timestamp.
            </p>
            {enablePITR.isError ? (
              <p className="text-sm text-destructive">
                {enablePITR.error.message}
              </p>
            ) : null}
            <div>
              <Button
                type="button"
                disabled={enablePITR.isPending}
                onClick={() => {
                  enablePITR.mutate(undefined, {
                    onSuccess: () => {
                      toast.add({
                        title: 'Point-in-time restore enabled.',
                        description:
                          'Take a base backup below once the database restarts with archiving on.',
                        type: 'success',
                      })
                    },
                  })
                }}
              >
                {enablePITR.isPending ? 'Enabling...' : 'Enable'}
              </Button>
            </div>
          </div>
        ) : null}

        {status.data?.enabled ? (
          <EnabledPITRContent databaseName={databaseName} />
        ) : null}
      </CardContent>
    </Card>
  )
}

function EnabledPITRContent({ databaseName }: { databaseName: string }) {
  const status = usePITRStatus(databaseName)
  const baseBackups = useBaseBackupHistory(databaseName)
  const triggerBaseBackup = useTriggerBaseBackup(databaseName)
  const restoreHistory = usePITRRestoreHistory(databaseName)

  const pitrWindow = status.data
  const succeededBackups = (baseBackups.data ?? []).filter(
    (b) => b.status === 'succeeded',
  )

  return (
    <div className="flex flex-col gap-5">
      <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        <div>
          <dt className="text-xs text-muted-foreground uppercase">
            Enabled since
          </dt>
          <dd className="mt-1 text-sm text-foreground">
            {formatDate(pitrWindow?.enabled_at, 'unknown')}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground uppercase">
            Recoverable window
          </dt>
          <dd className="mt-1 text-sm text-foreground">
            {pitrWindow?.window_error ? (
              <span className="text-destructive">
                {pitrWindow.window_error}
              </span>
            ) : pitrWindow?.has_base_backup && pitrWindow.window_start ? (
              <>
                {formatDate(pitrWindow.window_start, '-')} &ndash;{' '}
                {formatDate(pitrWindow.window_end, '-')}
              </>
            ) : (
              <span className="text-muted-foreground italic">
                no base backup yet
              </span>
            )}
          </dd>
        </div>
      </dl>

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium text-foreground">Base backups</h3>
        <TriggerBackupRowView
          pickerId="base-backup-target-picker"
          trigger={triggerBaseBackup}
        />
        <BaseBackupHistoryTableView
          history={baseBackups.data ?? []}
          isLoading={baseBackups.isLoading}
        />
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-foreground">
            Restore to a point in time
          </h3>
          <PITRRestoreDialog
            databaseName={databaseName}
            pitrWindow={pitrWindow}
            succeededBackups={succeededBackups}
          />
        </div>
        <PITRRestoreHistoryTableView
          history={restoreHistory.data ?? []}
          isLoading={restoreHistory.isLoading}
        />
      </div>
    </div>
  )
}

function BaseBackupHistoryTableView({
  history,
  isLoading,
}: {
  history: BaseBackupHistoryRecord[]
  isLoading: boolean
}) {
  if (isLoading || history.length === 0) {
    return null
  }
  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Status</TableHead>
            <TableHead>Size</TableHead>
            <TableHead>Started</TableHead>
            <TableHead>Finished</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {history.map((record) => (
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
                {record.size_bytes > 0
                  ? `${(record.size_bytes / 1024 / 1024).toFixed(1)} MiB`
                  : '-'}
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
  )
}

function PITRRestoreHistoryTableView({
  history,
  isLoading,
}: {
  history: {
    id: string
    status: string
    error?: string
    target_timestamp: string
    started_at: string
    finished_at?: string
  }[]
  isLoading: boolean
}) {
  if (isLoading || history.length === 0) {
    return null
  }
  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Status</TableHead>
            <TableHead>Target time</TableHead>
            <TableHead>Started</TableHead>
            <TableHead>Finished</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {history.map((record) => (
            <TableRow key={record.id}>
              <TableCell>
                <div className="flex flex-col gap-1">
                  <StatusBadge
                    status={record.status as 'running' | 'succeeded' | 'failed'}
                  />
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
                {formatDate(record.target_timestamp, '-')}
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
  )
}

// isoToLocalInputValue/localInputValueToISO convert between an
// <input type="datetime-local"> value (no timezone, local wall-clock
// time) and an RFC3339 UTC string the API expects: the picker always
// shows and accepts the viewer's own local time, translated at the
// boundary, the same way every other timestamp display in this app
// already defers to Date's own locale formatting (formatDate).
function isoToLocalInputValue(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function localInputValueToISO(value: string): string | null {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) {
    return null
  }
  return d.toISOString()
}

function PITRRestoreDialog({
  databaseName,
  pitrWindow,
  succeededBackups,
}: {
  databaseName: string
  pitrWindow:
    | { has_base_backup: boolean; window_start?: string; window_end?: string }
    | undefined
  succeededBackups: BaseBackupHistoryRecord[]
}) {
  const [open, setOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [targetLocal, setTargetLocal] = useState('')
  const triggerRestore = useTriggerPITRRestore(databaseName)

  const canRestore = Boolean(
    pitrWindow?.has_base_backup &&
    pitrWindow.window_start &&
    pitrWindow.window_end,
  )

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setConfirmText('')
      setTargetLocal('')
      triggerRestore.reset()
    }
  }

  const targetISO = targetLocal ? localInputValueToISO(targetLocal) : null

  // The most recent succeeded base backup at or before the chosen
  // target: any succeeded base backup taken before the target works
  // (WAL archiving is continuous for the whole time PITR has been on),
  // but the most recent one minimizes how much WAL Postgres has to
  // replay to reach it.
  const chosenBackup = useMemo(() => {
    if (!targetISO) return null
    const targetMs = new Date(targetISO).getTime()
    return (
      succeededBackups
        .filter((b) => new Date(b.started_at).getTime() <= targetMs)
        .sort(
          (a, b) =>
            new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
        )[0] ?? null
    )
  }, [succeededBackups, targetISO])

  const confirmed = confirmText === databaseName

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button variant="outline" size="sm" disabled={!canRestore} />}
      >
        <ClockCounterClockwiseIcon className="size-3.5" aria-hidden="true" />
        Restore to timestamp
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Restore &ldquo;{databaseName}&rdquo; to a point in time?
          </DialogTitle>
          <DialogDescription>
            This overwrites &ldquo;{databaseName}&rdquo;&apos;s current data
            with its contents at the chosen timestamp. Anything written after
            that moment is permanently lost. This cannot be undone.
          </DialogDescription>
        </DialogHeader>

        <Field>
          <FieldLabel htmlFor="pitr-target-time">Restore to</FieldLabel>
          <Input
            id="pitr-target-time"
            type="datetime-local"
            step={1}
            min={
              pitrWindow?.window_start
                ? isoToLocalInputValue(pitrWindow.window_start)
                : undefined
            }
            max={
              pitrWindow?.window_end
                ? isoToLocalInputValue(pitrWindow.window_end)
                : undefined
            }
            value={targetLocal}
            onChange={(event) => {
              setTargetLocal(event.target.value)
            }}
          />
          <FieldDescription>
            {pitrWindow?.window_start && pitrWindow.window_end ? (
              <>
                Recoverable between {formatDate(pitrWindow.window_start, '-')}{' '}
                and {formatDate(pitrWindow.window_end, '-')}.
              </>
            ) : null}
          </FieldDescription>
        </Field>

        {targetISO && chosenBackup ? (
          <p className="text-xs text-muted-foreground">
            Replaying from base backup taken{' '}
            {formatDate(chosenBackup.started_at, 'unknown time')}.
          </p>
        ) : null}
        {targetISO && !chosenBackup ? (
          <p className="text-xs text-destructive">
            No succeeded base backup exists at or before that time.
          </p>
        ) : null}

        <Field>
          <FieldLabel htmlFor="pitr-restore-confirm-name">
            Type &ldquo;{databaseName}&rdquo; to confirm
          </FieldLabel>
          <Input
            id="pitr-restore-confirm-name"
            autoComplete="off"
            spellCheck={false}
            value={confirmText}
            onChange={(event) => {
              setConfirmText(event.target.value)
            }}
          />
        </Field>

        {triggerRestore.isError ? (
          <p className="text-sm text-destructive">
            {triggerRestore.error.message}
          </p>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={
              !confirmed ||
              !targetISO ||
              !chosenBackup ||
              triggerRestore.isPending
            }
            onClick={() => {
              if (!targetISO || !chosenBackup) return
              triggerRestore.mutate(
                { base_backup_id: chosenBackup.id, target_time: targetISO },
                {
                  onSuccess: () => {
                    setOpen(false)
                    setConfirmText('')
                    setTargetLocal('')
                    toast.add({
                      title: 'Point-in-time restore started.',
                      description:
                        'This list updates automatically once it finishes.',
                      type: 'success',
                    })
                  },
                  onError: (error: ApiError) => {
                    toast.add({
                      title: 'Could not start restore.',
                      description: error.message,
                      type: 'error',
                    })
                  },
                },
              )
            }}
          >
            {triggerRestore.isPending ? 'Starting...' : 'Restore database'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
