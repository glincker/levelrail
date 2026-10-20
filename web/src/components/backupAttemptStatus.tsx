import type { VariantProps } from 'class-variance-authority'
import type { Icon } from '@phosphor-icons/react'
import {
  CheckCircleIcon,
  SpinnerIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { BackupStatus } from '../types/backupHistory'
import type { RestoreStatus } from '../types/restoreHistory'

// Shared between BackupsSection's backup and restore history tables: both
// status unions (BackupStatus, RestoreStatus) are the identical three
// literal strings, running/succeeded/failed, one attempt lifecycle
// reused for two different resources (see store.RestoreHistory's own
// Go-side doc comment for why the backend reuses the same status
// constants rather than defining a second, identically-valued set), so
// one badge/label/icon mapping and one StatusBadge component here covers
// both tables rather than duplicating all three per table.
export type AttemptStatus = BackupStatus | RestoreStatus

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

export function StatusBadge({ status }: { status: AttemptStatus }) {
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
