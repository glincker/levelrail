// Wire types for the backup target resource, matching
// internal/api/backup_targets.go's backupTargetResource /
// createBackupTargetRequest exactly. Credentials (access_key_id,
// secret_access_key) are write-only, present on the create request and
// nowhere else: handleCreateBackupTarget's own doc comment is explicit
// that they are never echoed back, the same "a secret's value and a
// resource's other fields have different write (and read) paths"
// reasoning tokens.go's plaintext-once token establishes elsewhere.

export type BackupProvider = 'aws' | 'r2' | 'custom'

export const BACKUP_PROVIDERS: BackupProvider[] = ['aws', 'r2', 'custom']

export interface BackupTarget {
  id: string
  name: string
  provider: BackupProvider
  // Omitted for aws (backupTargetResource's `omitempty` on Endpoint),
  // present for r2/custom.
  endpoint?: string
  region?: string
  bucket: string
  created_at: string
}

export interface CreateBackupTargetRequest {
  name: string
  provider: BackupProvider
  // Required for r2/custom, must be omitted for aws
  // (validateCreateBackupTargetRequest's own check).
  endpoint?: string
  region?: string
  bucket: string
  access_key_id: string
  secret_access_key: string
}
