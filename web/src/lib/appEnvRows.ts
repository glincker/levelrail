import type { AppEnvDiff } from './appEnvDiff'

export type EnvRowStatus = 'added' | 'removed' | 'changed'

export interface EnvRow {
  key: string
  status: EnvRowStatus
  secret: boolean
  a?: string
  b?: string
}

/** toRows flattens a diff into key-sorted rows; secret rows carry no values. */
export function toRows(diff: AppEnvDiff): EnvRow[] {
  const rows: EnvRow[] = [
    ...diff.onlyA.map((e): EnvRow => ({ ...e, status: 'added' })),
    ...diff.onlyB.map((e): EnvRow => ({ ...e, status: 'removed' })),
    ...diff.differ.map((e): EnvRow => ({ ...e, status: 'changed' })),
  ]
  return rows.sort((x, y) => x.key.localeCompare(y.key))
}
