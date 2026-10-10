import type {
  DbCell,
  DbColumnFilter,
  DbFilterOp,
} from '../types/databaseViewer'

export type ColumnKind =
  | 'number'
  | 'text'
  | 'boolean'
  | 'datetime'
  | 'json'
  | 'uuid'
  | 'binary'
  | 'other'

export type SqlDialect = 'postgres' | 'mysql'

const NUMERIC = /^-?\d+(\.\d+)?([eE][+-]?\d+)?$/

export function columnKind(type: string): ColumnKind {
  const t = type.toLowerCase()
  if (t.includes('json')) return 'json'
  if (t.includes('uuid')) return 'uuid'
  if (/^(bool|tinyint\(1\))/.test(t)) return 'boolean'
  if (/(timestamp|datetime|^date|^time|interval|year)/.test(t))
    return 'datetime'
  if (/(blob|bytea|binary|varbinary)/.test(t)) return 'binary'
  if (
    /(int|serial|numeric|decimal|float|double|real|money|number)/.test(t) &&
    !t.includes('interval') &&
    !t.includes('point')
  ) {
    return 'number'
  }
  if (/(char|text|citext|name|enum|set)/.test(t)) return 'text'
  return 'other'
}

export function dialectOf(engine: string): SqlDialect {
  return engine === 'postgres' ? 'postgres' : 'mysql'
}

export function quoteIdentifier(dialect: SqlDialect, name: string): string {
  if (dialect === 'postgres') return `"${name.replace(/"/g, '""')}"`
  return `\`${name.replace(/`/g, '``')}\``
}

export function qualifiedName(
  dialect: SqlDialect,
  schema: string,
  table: string,
): string {
  return `${quoteIdentifier(dialect, schema)}.${quoteIdentifier(dialect, table)}`
}

function csvField(cell: DbCell): string {
  if (cell === null) return ''
  if (cell === '' || /[",\r\n]/.test(cell)) {
    return `"${cell.replace(/"/g, '""')}"`
  }
  return cell
}

// An empty string is written as "" and NULL as nothing, so the two stay
// distinguishable in the exported file.
export function toCsv(columns: string[], rows: DbCell[][]): string {
  const lines = [columns.map((c) => csvField(c)).join(',')]
  for (const row of rows) {
    lines.push(columns.map((_, i) => csvField(row[i] ?? null)).join(','))
  }
  return lines.join('\n')
}

function jsonValue(cell: DbCell, kind: ColumnKind): unknown {
  if (cell === null) return null
  if (kind === 'number' && NUMERIC.test(cell)) return Number(cell)
  if (kind === 'boolean') {
    if (cell === 't' || cell === 'true' || cell === '1') return true
    if (cell === 'f' || cell === 'false' || cell === '0') return false
  }
  if (kind === 'json') {
    try {
      return JSON.parse(cell) as unknown
    } catch {
      return cell
    }
  }
  return cell
}

export function toJson(
  columns: string[],
  rows: DbCell[][],
  kinds: ColumnKind[] = [],
): string {
  const objects = rows.map((row) => {
    const o: Record<string, unknown> = {}
    columns.forEach((c, i) => {
      o[c] = jsonValue(row[i] ?? null, kinds[i] ?? 'text')
    })
    return o
  })
  return JSON.stringify(objects, null, 2)
}

function sqlLiteral(
  cell: DbCell,
  kind: ColumnKind,
  dialect: SqlDialect,
): string {
  if (cell === null) return 'NULL'
  if (kind === 'number' && NUMERIC.test(cell)) return cell
  if (kind === 'boolean' && dialect === 'postgres') {
    if (cell === 't' || cell === 'true') return 'TRUE'
    if (cell === 'f' || cell === 'false') return 'FALSE'
  }
  let escaped = cell.replace(/'/g, "''")
  if (dialect === 'mysql') escaped = escaped.replace(/\\/g, '\\\\')
  return `'${escaped}'`
}

export function toInsert(
  dialect: SqlDialect,
  schema: string,
  table: string,
  columns: string[],
  rows: DbCell[][],
  kinds: ColumnKind[] = [],
): string {
  const cols = columns.map((c) => quoteIdentifier(dialect, c)).join(', ')
  const target = qualifiedName(dialect, schema, table)
  return rows
    .map((row) => {
      const values = columns
        .map((_, i) => sqlLiteral(row[i] ?? null, kinds[i] ?? 'text', dialect))
        .join(', ')
      return `INSERT INTO ${target} (${cols}) VALUES (${values});`
    })
    .join('\n')
}

export function selectStatement(
  dialect: SqlDialect,
  schema: string,
  table: string,
  limit: number,
): string {
  return `SELECT * FROM ${qualifiedName(dialect, schema, table)} LIMIT ${limit};`
}

export function prettyJson(value: string): string | undefined {
  const trimmed = value.trim()
  if (!/^[{["]|^(true|false|null)$|^-?\d/.test(trimmed)) return undefined
  try {
    const parsed: unknown = JSON.parse(trimmed)
    if (parsed === null || typeof parsed !== 'object') return undefined
    return JSON.stringify(parsed, null, 2)
  } catch {
    return undefined
  }
}

export const LONG_CELL_CHARS = 80

export function needsDetail(cell: DbCell, kind: ColumnKind): boolean {
  if (cell === null) return false
  return (
    cell.length > LONG_CELL_CHARS ||
    cell.includes('\n') ||
    (kind === 'json' && cell.length > 2)
  )
}

const EQUALS_PREFIX = '='
const NULL_TOKEN = 'is:null'
const NOT_NULL_TOKEN = 'not:null'

// A column filter box accepts plain text (contains), "=text" (exact),
// "is:null" and "not:null".
export function parseFilterInput(
  column: string,
  raw: string,
): DbColumnFilter | null {
  const text = raw.trim()
  if (text === '') return null
  const lower = text.toLowerCase()
  if (lower === NULL_TOKEN) return { column, op: 'is_null', value: '' }
  if (lower === NOT_NULL_TOKEN) return { column, op: 'not_null', value: '' }
  const op: DbFilterOp = text.startsWith(EQUALS_PREFIX) ? 'equals' : 'contains'
  const value = op === 'equals' ? text.slice(EQUALS_PREFIX.length) : text
  return { column, op, value }
}
