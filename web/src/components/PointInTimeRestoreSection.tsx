import { lazy, Suspense, useMemo, useState } from 'react'
import {
  InfoGlassIcon,
  RestoreGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { formatDate } from '../lib/format'
import { isoToLocalInputValue, localInputValueToISO } from '../lib/pitrTime'
import { PITRBinlogCopy } from './PITRBinlogCopy'
import { WalShipStatusItem } from './WalShipStatusItem'
import { ApiError } from '../lib/apiError'
import {
  BaseBackupHistoryTableView,
  PITRRestoreHistoryTableView,
} from './PITRHistoryTables'
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

// MySQL and MariaDB restore into a new database, so their dialog is a separate chunk.
const PITRRestoreIntoNewDialog = lazy(() =>
  import('./PITRRestoreIntoNewDialog').then((m) => ({
    default: m.PITRRestoreIntoNewDialog,
  })),
)

const PITR_ENGINES = new Set(['postgres', 'mysql', 'mariadb'])
const NEW_DATABASE_ENGINES = new Set(['mysql', 'mariadb'])

// Point-in-time restore (internal/api/pitr.go) for Postgres (WAL archiving,
// restores in place) and MySQL or MariaDB (binlog archiving, restores into
// a new database): a
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
  if (!PITR_ENGINES.has(database.engine)) {
    return null
  }
  return (
    <PointInTimeRestoreCard
      databaseName={database.name}
      engine={database.engine}
    />
  )
}

function PointInTimeRestoreCard({
  databaseName,
  engine,
}: {
  databaseName: string
  engine: string
}) {
  const status = usePITRStatus(databaseName)
  const enablePITR = useEnablePITR(databaseName)
  const disablePITR = useDisablePITR(databaseName)

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between">
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
          <div className="flex flex-col gap-3">
            <p className="text-sm text-muted-foreground">
              {NEW_DATABASE_ENGINES.has(engine) ? (
                <PITRBinlogCopy />
              ) : (
                'Enable continuous WAL archiving so this database can be restored to any exact moment, not just to when a backup happened to run.'
              )}
            </p>
            <p className="flex items-start gap-1.5 text-xs text-muted-foreground">
              <InfoGlassIcon
                className="mt-0.5 size-4 shrink-0 text-tone-info"
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
          <EnabledPITRContent databaseName={databaseName} engine={engine} />
        ) : null}
      </CardContent>
    </Card>
  )
}

function EnabledPITRContent({
  databaseName,
  engine,
}: {
  databaseName: string
  engine: string
}) {
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
        <WalShipStatusItem ship={pitrWindow?.wal_ship} />
      </dl>

      <div className="flex flex-col gap-2">
        <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
          Base backups
        </h3>
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
          <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            Restore to a point in time
          </h3>
          {NEW_DATABASE_ENGINES.has(engine) ? (
            <Suspense fallback={null}>
              <PITRRestoreIntoNewDialog
                databaseName={databaseName}
                engine={engine}
                pitrWindow={pitrWindow}
                succeededBackups={succeededBackups}
              />
            </Suspense>
          ) : (
            <PITRRestoreDialog
              databaseName={databaseName}
              pitrWindow={pitrWindow}
              succeededBackups={succeededBackups}
            />
          )}
        </div>
        <PITRRestoreHistoryTableView
          history={restoreHistory.data ?? []}
          isLoading={restoreHistory.isLoading}
        />
      </div>
    </div>
  )
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
        <RestoreGlassIcon className="size-4" aria-hidden="true" />
        Restore to timestamp
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <TriangleAlertGlassIcon
              className="size-4.5 text-tone-warning"
              aria-hidden="true"
            />
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
