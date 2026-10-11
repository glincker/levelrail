import type { ConditionStatus, ReconcileCondition } from '../types/deploy'

export type ConditionOutcome = 'ok' | 'failing' | 'pending'

export interface HumanCondition {
  outcome: ConditionOutcome
  title: string
  detail: string
  raw: string
}

const OUTCOME: Record<ConditionStatus, ConditionOutcome> = {
  True: 'ok',
  False: 'failing',
  Unknown: 'pending',
}

// "AlreadyRunning" -> "Already running", "TLSReady" -> "TLS ready".
export function humanizeIdentifier(id: string): string {
  const spaced = id
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
    .trim()
  if (spaced === '') return ''
  const words = spaced.split(/\s+/)
  return words
    .map((w, i) => (i === 0 || /^[A-Z0-9]+$/.test(w) ? w : w.toLowerCase()))
    .join(' ')
}

// Raw reconcile vocabulary ("Ready: AlreadyRunning") becomes a sentence the
// operator can read; the untouched string is kept for the details view.
export function humanizeCondition(c: ReconcileCondition): HumanCondition {
  const outcome = OUTCOME[c.Status]
  return {
    outcome,
    title: humanizeIdentifier(c.Type),
    detail: humanizeIdentifier(c.Reason),
    raw: `${c.Type}: ${c.Reason} (${c.Status})`,
  }
}
