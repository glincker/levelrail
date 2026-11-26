import type { JsonValue } from '../types/databaseMongo'

export type MongoJsonKind = 'filter' | 'sort' | 'projection'

export type MongoJsonError =
  'syntax' | 'notObject' | 'sortDirection' | 'projectionValue'

export type MongoJsonCheck =
  | { ok: true; value: JsonValue | undefined }
  | { ok: false; error: MongoJsonError }

function isObject(v: JsonValue): v is { [key: string]: JsonValue } {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

// Checks one box of the query bar. Empty means "not set"; the server still
// applies its own operator allow list and reports rejections itself.
export function checkMongoJson(
  text: string,
  kind: MongoJsonKind,
): MongoJsonCheck {
  const trimmed = text.trim()
  if (trimmed === '') return { ok: true, value: undefined }
  let parsed: JsonValue
  try {
    parsed = JSON.parse(trimmed) as JsonValue
  } catch {
    return { ok: false, error: 'syntax' }
  }
  if (!isObject(parsed)) return { ok: false, error: 'notObject' }
  const values = Object.values(parsed)
  if (kind === 'sort' && values.some((v) => v !== 1 && v !== -1)) {
    return { ok: false, error: 'sortDirection' }
  }
  if (
    kind === 'projection' &&
    values.some((v) => typeof v === 'string' || v === null)
  ) {
    return { ok: false, error: 'projectionValue' }
  }
  return { ok: true, value: parsed }
}

export interface EjsonScalar {
  label: string
  text: string
}

const EJSON_LABELS: Record<string, string> = {
  $oid: 'ObjectId',
  $numberLong: 'Long',
  $numberInt: 'Int32',
  $numberDouble: 'Double',
  $numberDecimal: 'Decimal',
  $uuid: 'UUID',
}

// Collapses single-key extended JSON wrappers so ids and dates read as
// scalars instead of one-field objects.
export function ejsonScalar(value: JsonValue): EjsonScalar | null {
  if (!isObject(value)) return null
  const keys = Object.keys(value)
  if (keys.length !== 1) return null
  const key = keys[0] ?? ''
  const inner = value[key] ?? null
  if (key === '$date') {
    if (typeof inner === 'string') return { label: 'Date', text: inner }
    if (isObject(inner) && typeof inner.$numberLong === 'string') {
      const ms = Number(inner.$numberLong)
      const iso = Number.isFinite(ms) ? new Date(ms).toISOString() : null
      return { label: 'Date', text: iso ?? inner.$numberLong }
    }
    return null
  }
  const label = EJSON_LABELS[key]
  if (label && (typeof inner === 'string' || typeof inner === 'number')) {
    return { label, text: String(inner) }
  }
  return null
}

export function isContainer(
  value: JsonValue,
): value is JsonValue[] | { [key: string]: JsonValue } {
  return (
    typeof value === 'object' && value !== null && ejsonScalar(value) === null
  )
}

export function documentId(doc: JsonValue): JsonValue | undefined {
  return isObject(doc) ? doc._id : undefined
}

export function documentTitle(doc: JsonValue): string {
  const id = documentId(doc)
  if (id === undefined) return ''
  const scalar = ejsonScalar(id)
  return scalar ? scalar.text : JSON.stringify(id)
}

export function prettyDocument(doc: JsonValue): string {
  return JSON.stringify(doc, null, 2)
}

export function formatIndexKey(key: Record<string, JsonValue>): string {
  return Object.entries(key)
    .map(([field, dir]) => `${field}: ${JSON.stringify(dir)}`)
    .join(', ')
}
