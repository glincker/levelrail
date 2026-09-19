// Resource-kind-dispatching wrappers around the verification badge,
// download link, and delete dialog already built per-resource
// (BackupVerificationBadge/VolumeBackupVerificationBadge,
// DownloadBackupLinkView, DeleteBackupDialog/DeleteVolumeBackupDialog):
// routes/backups/index.tsx renders one merged table across both resource
// kinds, so each row needs to route to whichever pair of components
// actually owns its own backup, rather than duplicating either
// component's logic a third time.

import { BackupVerificationBadge } from './BackupVerificationBadge'
import { VolumeBackupVerificationBadge } from './VolumeBackupVerificationBadge'
import { DownloadBackupLinkView } from './BackupsSection'
import {
  DeleteBackupDialog,
  DeleteVolumeBackupDialog,
} from './DeleteBackupDialog'
import { backupDownloadURL } from '../queries/backupHistory'
import { volumeBackupDownloadURL } from '../queries/volumeBackupHistory'
import type { BackupHistoryRecord } from '../types/backupHistory'

export function AnyBackupVerificationBadge({
  backup,
}: {
  backup: BackupHistoryRecord
}) {
  if (backup.resource_kind === 'volume') {
    return (
      <VolumeBackupVerificationBadge
        appName={backup.service_name ?? ''}
        volumeName={backup.volume_name ?? ''}
        backup={backup}
      />
    )
  }
  return (
    <BackupVerificationBadge
      databaseName={backup.database_name ?? ''}
      backup={backup}
    />
  )
}

export function AnyBackupDownloadLink({
  backup,
}: {
  backup: BackupHistoryRecord
}) {
  const href =
    backup.resource_kind === 'volume'
      ? volumeBackupDownloadURL(
          backup.service_name ?? '',
          backup.volume_name ?? '',
          backup.id,
        )
      : backupDownloadURL(backup.database_name ?? '', backup.id)
  return <DownloadBackupLinkView href={href} />
}

export function AnyBackupDeleteDialog({
  backup,
}: {
  backup: BackupHistoryRecord
}) {
  if (backup.resource_kind === 'volume') {
    return (
      <DeleteVolumeBackupDialog
        appName={backup.service_name ?? ''}
        volumeName={backup.volume_name ?? ''}
        backup={backup}
      />
    )
  }
  return (
    <DeleteBackupDialog
      databaseName={backup.database_name ?? ''}
      backup={backup}
    />
  )
}
