import {
  ShieldCheckIcon,
  ShieldWarningIcon,
  SpinnerIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useLatestBackupVerification,
  useVerifyBackup,
} from '../queries/backupVerification'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One succeeded backup's verification status plus a "Verify" action,
// rendered alongside DownloadBackupLink/RestoreBackupDialog in
// BackupsSection's history table row. Re-downloads the stored object and
// checks it for corruption (checksum, size, a lightweight structural
// check, see internal/backup.VerifyRunner's own doc comment);
// deliberately never a live restore, so this is safe to click at any
// time without risking the database it was taken from.
export function BackupVerificationBadge({
  databaseName,
  backup,
}: {
  databaseName: string
  backup: BackupHistoryRecord
}) {
  const { data } = useLatestBackupVerification(
    databaseName,
    backup.id,
    backup.status === 'succeeded',
  )
  const verify = useVerifyBackup(databaseName, backup.id)
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
      <VerificationStatusBadge status={latest?.status} error={latest?.error} />
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

function VerificationStatusBadge({
  status,
  error,
}: {
  status?: 'running' | 'passed' | 'failed'
  error?: string
}) {
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
        Verifying...
      </Badge>
    )
  }
  if (status === 'failed') {
    return (
      <Badge
        variant="destructive"
        className="rounded-full"
        title={error}
      >
        <ShieldWarningIcon className="size-3" aria-hidden="true" />
        Failed verification
      </Badge>
    )
  }
  return (
    <Badge variant="success" className="rounded-full">
      <ShieldCheckIcon className="size-3" aria-hidden="true" />
      Verified
    </Badge>
  )
}
