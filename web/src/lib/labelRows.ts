import type { NodeLabels } from '../types/nodeMoves'

export interface LabelRow {
  id: number
  key: string
  value: string
}

export type LabelRowsError = 'duplicate' | 'incomplete'

let nextRowId = 1

export function newLabelRow(key = '', value = ''): LabelRow {
  nextRowId += 1
  return { id: nextRowId, key, value }
}

export function rowsFromLabels(labels: NodeLabels): LabelRow[] {
  return Object.keys(labels)
    .sort()
    .map((key) => newLabelRow(key, labels[key] ?? ''))
}

// Blank rows are dropped, a half-filled row or a repeated key is an error
// the form shows before anything is sent.
export function labelsFromRows(rows: LabelRow[]): {
  labels: NodeLabels
  error: LabelRowsError | null
} {
  const labels: NodeLabels = {}
  for (const row of rows) {
    const key = row.key.trim()
    const value = row.value.trim()
    if (key === '' && value === '') {
      continue
    }
    if (key === '' || value === '') {
      return { labels, error: 'incomplete' }
    }
    if (key in labels) {
      return { labels, error: 'duplicate' }
    }
    labels[key] = value
  }
  return { labels, error: null }
}

export function sameLabels(a: NodeLabels, b: NodeLabels): boolean {
  const aKeys = Object.keys(a)
  if (aKeys.length !== Object.keys(b).length) {
    return false
  }
  return aKeys.every((k) => b[k] === a[k])
}
