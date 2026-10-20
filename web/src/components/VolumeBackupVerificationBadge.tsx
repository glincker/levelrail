import { toast } from '@/components/ui/toast'
import {
  useLatestVolumeBackupVerification,
  useVerifyVolumeBackup,
} from '../queries/volumeBackupVerification'
import { VerificationBadgeAction } from './BackupVerificationBadge'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One succeeded app service volume backup's verification status plus a
// "Verify" action, mirroring BackupVerificationBadge's exact shape for
// the database resource kind via the shared VerificationBadgeAction.
// Re-downloads the stored archive and checks it for corruption (checksum,
// size, non-empty), never a live restore.
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
    <VerificationBadgeAction
      status={latest?.status}
      error={latest?.error}
      checkedBy={latest?.checked_by}
      disabled={verify.isPending || latest?.status === 'running'}
      onVerify={handleVerify}
    />
  )
}
