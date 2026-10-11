export type DiffValue = string | number | boolean | null

export interface DiffRow {
  key: string
  before: DiffValue | undefined
  after: DiffValue | undefined
  changed: boolean
}

/** Rows for every key in either side, in first-seen order. */
export function diffRows(
  before: Readonly<Record<string, DiffValue>>,
  after: Readonly<Record<string, DiffValue>>,
): DiffRow[] {
  const keys = [...new Set([...Object.keys(before), ...Object.keys(after)])]
  return keys.map((key) => ({
    key,
    before: before[key],
    after: after[key],
    changed: before[key] !== after[key],
  }))
}
