import { useTriggerVolumeRestore } from '../queries/volumeRestoreHistory'
import { ConfirmRestoreDialog } from './RestoreBackupDialog'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One succeeded app service volume backup's restore action, mirroring
// RestoreBackupDialog's exact shape for the database resource kind via
// the shared ConfirmRestoreDialog. A volume has no single global name the
// way a database does, so the string to type is "<app>/<volume>", the
// same composite identifier the CLI's own --confirm flag requires
// (cmd/levelrail-cli/app_volume_backups_restore.go).
export function RestoreVolumeBackupDialog({
  appName,
  volumeName,
  backup,
}: {
  appName: string
  volumeName: string
  backup: BackupHistoryRecord
}) {
  const target = `${appName}/${volumeName}`
  const triggerRestore = useTriggerVolumeRestore(appName, volumeName)
  return (
    <ConfirmRestoreDialog
      target={target}
      confirmInputId="volume-restore-confirm-name"
      subjectLabel="volume contents"
      confirmButtonLabel="Restore volume"
      backupId={backup.id}
      triggerRestore={triggerRestore}
    />
  )
}
