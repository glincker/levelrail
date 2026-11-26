import type { DbCell } from '../../types/databaseViewer'

export type InferredType = 'int' | 'number' | 'bool' | 'json' | 'text' | 'null'

// The API returns every cell as a string and no column types, so the chip
// is a guess from the loaded values and is labelled as such in the UI.
export function inferColumnType(rows: DbCell[][], col: number): InferredType {
  let type: InferredType | null = null
  const merge = (next: InferredType) => {
    if (type === null || type === next) {
      type = next
    } else if (
      (type === 'int' && next === 'number') ||
      (type === 'number' && next === 'int')
    ) {
      type = 'number'
    } else {
      type = 'text'
    }
  }
  for (const row of rows) {
    const v = row[col]
    if (v === null || v === undefined) continue
    if (/^-?\d+$/.test(v)) merge('int')
    else if (/^-?\d*\.\d+([eE][+-]?\d+)?$/.test(v)) merge('number')
    else if (/^(true|false|t|f)$/i.test(v)) merge('bool')
    else if (/^[[{]/.test(v) && isJson(v)) merge('json')
    else return 'text'
  }
  return type ?? 'null'
}

function isJson(v: string): boolean {
  try {
    JSON.parse(v)
    return true
  } catch {
    return false
  }
}
