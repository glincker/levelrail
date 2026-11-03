import type {
  NodeTaint,
  PlacementToleration,
  TaintEffect,
  TolerationOperator,
} from '../types/nodeMoves'

export const TAINT_EFFECTS: TaintEffect[] = [
  'NoSchedule',
  'PreferNoSchedule',
  'NoExecute',
]

export interface TaintRow {
  id: number
  key: string
  value: string
  effect: TaintEffect
  operator: TolerationOperator
}

export type TaintRowsError = 'incomplete' | 'duplicate'

let nextRowId = 1

export function newTaintRow(
  key = '',
  value = '',
  effect: TaintEffect = 'NoSchedule',
  operator: TolerationOperator = 'Equal',
): TaintRow {
  nextRowId += 1
  return { id: nextRowId, key, value, effect, operator }
}

export function rowsFromTaints(taints: NodeTaint[]): TaintRow[] {
  return taints.map((x) => newTaintRow(x.key, x.value ?? '', x.effect))
}

export function rowsFromTolerations(tols: PlacementToleration[]): TaintRow[] {
  return tols.map((x) =>
    newTaintRow(
      x.key ?? '',
      x.value ?? '',
      x.effect ?? 'NoSchedule',
      x.operator === 'Exists' ? 'Exists' : 'Equal',
    ),
  )
}

// A blank row is dropped, a row without a key is an error, and one key and
// effect pair may appear once.
export function taintsFromRows(rows: TaintRow[]): {
  taints: NodeTaint[]
  error: TaintRowsError | null
} {
  const taints: NodeTaint[] = []
  const seen = new Set<string>()
  for (const row of rows) {
    const key = row.key.trim()
    const value = row.value.trim()
    if (key === '' && value === '') {
      continue
    }
    if (key === '') {
      return { taints, error: 'incomplete' }
    }
    const id = `${key}:${row.effect}`
    if (seen.has(id)) {
      return { taints, error: 'duplicate' }
    }
    seen.add(id)
    taints.push({ key, value, effect: row.effect })
  }
  return { taints, error: null }
}

export function tolerationsFromRows(rows: TaintRow[]): {
  tolerations: PlacementToleration[]
  error: TaintRowsError | null
} {
  const out: PlacementToleration[] = []
  for (const row of rows) {
    const key = row.key.trim()
    const value = row.operator === 'Exists' ? '' : row.value.trim()
    if (key === '' && value === '') {
      continue
    }
    if (key === '') {
      return { tolerations: out, error: 'incomplete' }
    }
    out.push({ key, operator: row.operator, value, effect: row.effect })
  }
  return { tolerations: out, error: null }
}

export function sameJSON(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

export function normalizeTaints(taints: NodeTaint[]): NodeTaint[] {
  return taints.map((x) => ({
    key: x.key,
    value: x.value ?? '',
    effect: x.effect,
  }))
}

export function normalizeTolerations(
  tols: PlacementToleration[],
): PlacementToleration[] {
  return tols.map((x) => ({
    key: x.key ?? '',
    operator: x.operator === 'Exists' ? 'Exists' : 'Equal',
    value: x.value ?? '',
    effect: x.effect ?? 'NoSchedule',
  }))
}
