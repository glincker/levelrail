import type { EmailProvider } from '../../queries/emailProviders'

export type ProviderHealth = 'healthy' | 'failing' | 'paused' | 'untested'

export function providerHealth(p: EmailProvider): ProviderHealth {
  if (p.enabled === false) return 'paused'
  if (p.last_error_kind) return 'failing'
  if (p.last_ok_at) return 'healthy'
  return 'untested'
}

// The API has no circuit flag: the backup is carrying mail when the primary is out and the backup sent more recently.
export function isRunningOnBackup(
  primary: EmailProvider | undefined,
  backup: EmailProvider | undefined,
): boolean {
  if (!primary || !backup || backup.enabled === false || !backup.last_ok_at) {
    return false
  }
  const primaryOut =
    primary.enabled === false || Boolean(primary.last_error_kind)
  if (!primaryOut) return false
  return (
    !primary.last_ok_at ||
    Date.parse(backup.last_ok_at) > Date.parse(primary.last_ok_at)
  )
}
