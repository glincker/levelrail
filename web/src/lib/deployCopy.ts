export interface DeployCopy {
  kind: 'rollback' | 'promote'
  /** Short id of the deploy this row re-serves, empty when the reason has none. */
  target: string
}

const TARGET_RE = /RollbackTo:\s*([\w-]+)/
const SHORT_TARGET = 12

// A promote or rollback re-serves an earlier build, so it has no build of its own.
export function deployCopyOf(attempt: {
  reason?: string
  trigger?: string
  source?: string
}): DeployCopy | null {
  const fromReason = TARGET_RE.exec(attempt.reason ?? '')
  const isRollback = attempt.trigger === 'rollback' || fromReason !== null
  if (!isRollback && attempt.source !== 'promote') return null
  return {
    kind: isRollback ? 'rollback' : 'promote',
    target: (fromReason?.[1] ?? '').slice(0, SHORT_TARGET),
  }
}

export type CopyLabelKey =
  | 'copyRow.rolledBack'
  | 'copyRow.rolledBackTo'
  | 'copyRow.promoted'
  | 'copyRow.promotedFrom'

export function copyLabelKey(copy: DeployCopy): CopyLabelKey {
  if (copy.kind === 'rollback') {
    return copy.target ? 'copyRow.rolledBackTo' : 'copyRow.rolledBack'
  }
  return copy.target ? 'copyRow.promotedFrom' : 'copyRow.promoted'
}
