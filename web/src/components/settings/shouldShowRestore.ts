import type { SelfUpgradeAttempt } from '../../queries/selfUpgrade'

// A skipped backup leaves nothing to restore from, so the command stays hidden.
export function shouldShowRestore(
  attempt: Pick<SelfUpgradeAttempt, 'outcome' | 'backup_name'> | undefined,
  command: string | undefined,
): boolean {
  return (
    attempt?.outcome === 'succeeded' &&
    attempt.backup_name !== '' &&
    Boolean(command)
  )
}
