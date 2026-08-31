import {
  ShieldCheckIcon,
  ShieldWarningIcon,
  SpinnerIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useLatestVolumeBackupVerification,
  useVerifyVolumeBackup,
} from '../queries/volumeBackupVerification'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One succeeded app service volume backup's verification status plus a
// "Verify" action, mirroring BackupVerificationBadge's exact shape for
// the database resource kind. Re-downloads the stored archive and checks
// it for corruption (checksum, size, non-empty), never a live restore.
export function VolumeBackupVerificationBadge({
  appName,
  volumeName,
  backup,
}: {
  appName: string
  volumeName: string
  backup: BackupHistoryRecord
}) {
  const { data } = useLatestVolumeBackupVerification(
    appName,
    volumeName,
    backup.id,
    backup.status === 'succeeded',
  )
  const verify = useVerifyVolumeBackup(appName, volumeName, backup.id)
  const latest = data?.[0]

  function handleVerify() {
    verify.mutate(undefined, {
      onSuccess: () => {
        toast.add({
          title: 'Verification started.',
          description: 'The badge updates automatically once it finishes.',
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not start verification.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="flex items-center gap-2">
      <VerificationStatusBadge
        status={latest?.status}
        error={latest?.error}
        checkedBy={latest?.checked_by}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={verify.isPending || latest?.status === 'running'}
        onClick={handleVerify}
      >
        Verify
      </Button>
    </div>
  )
}

// Mirrors internal/backup.ScheduledVerificationCheckedBy: the
// scheduler's sentinel value for a verification it ran automatically.
const AUTO_VERIFIED_CHECKED_BY = 'scheduler'

function VerificationStatusBadge({
  status,
  error,
  checkedBy,
}: {
  status?: 'running' | 'passed' | 'failed'
  error?: string
  checkedBy?: string
}) {
  const auto = checkedBy === AUTO_VERIFIED_CHECKED_BY

  if (!status) {
    return (
      <Badge variant="muted" className="rounded-full">
        Not yet verified
      </Badge>
    )
  }
  if (status === 'running') {
    return (
      <Badge variant="muted" className="rounded-full">
        <SpinnerIcon className="size-3 animate-spin" aria-hidden="true" />
        {auto ? 'Auto-verifying...' : 'Verifying...'}
      </Badge>
    )
  }
  if (status === 'failed') {
    return (
      <Badge variant="destructive" className="rounded-full" title={error}>
        <ShieldWarningIcon className="size-3" aria-hidden="true" />
        {auto ? 'Failed auto-verification' : 'Failed verification'}
      </Badge>
    )
  }
  return (
    <Badge
      variant="success"
      className="rounded-full"
      title={
        auto ? 'Verified automatically after the scheduled backup' : undefined
      }
    >
      <ShieldCheckIcon className="size-3" aria-hidden="true" />
      {auto ? 'Auto-verified' : 'Verified'}
    </Badge>
  )
}
