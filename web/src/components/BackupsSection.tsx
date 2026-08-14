import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import type { VariantProps } from 'class-variance-authority'
import type { Icon } from '@phosphor-icons/react'
import {
  CheckCircleIcon,
  CloudArrowUpIcon,
  SpinnerIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { useBackupHistory, useTriggerBackup } from '../queries/backupHistory'
import type { BackupHistoryRecord, BackupStatus } from '../types/backupHistory'

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

const STATUS_LABEL: Record<BackupStatus, string> = {
  running: 'Running',
  succeeded: 'Succeeded',
  failed: 'Failed',
}

const STATUS_BADGE_VARIANT: Record<
  BackupStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  running: 'muted',
  succeeded: 'success',
  failed: 'destructive',
}

const STATUS_ICON: Record<BackupStatus, Icon> = {
  running: SpinnerIcon,
  succeeded: CheckCircleIcon,
  failed: WarningCircleIcon,
}

function StatusBadge({ status }: { status: BackupStatus }) {
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

function BackupHistoryTable({ databaseName }: { databaseName: string }) {
  const { data: targets } = useBackupTargets()
  const { data, isLoading, error } = useBackupHistory(databaseName)
  const history = data ?? []

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
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Status</TableHead>
            <TableHead>Target</TableHead>
            <TableHead>Size</TableHead>
            <TableHead>Started</TableHead>
            <TableHead>Finished</TableHead>
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
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
      </CardContent>
    </Card>
  )
}
