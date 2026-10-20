// Shared provider display metadata for backup targets, used by both
// CreateBackupTargetDialog (the Select options) and BackupTargetTable
// (the per-row badge), so the two never drift on how 'aws' / 'r2' /
// 'custom' are labeled.

import type { BackupProvider } from '../types/backupTarget'

export const PROVIDER_LABEL: Record<BackupProvider, string> = {
  aws: 'AWS S3',
  r2: 'Cloudflare R2',
  custom: 'S3 compatible',
}
