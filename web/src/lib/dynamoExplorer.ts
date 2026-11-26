import type { DynamoAttr, DynamoItem } from '../types/databaseDynamo'

export type PlainValue =
  | string
  | number
  | boolean
  | null
  | PlainValue[]
  | { [key: string]: PlainValue }

// Turns one typed attribute such as {"S":"x"} into a plain JSON value.
export function plainAttr(attr: DynamoAttr): PlainValue {
  const [type, raw] = Object.entries(attr)[0] ?? ['NULL', true]
  switch (type) {
    case 'S':
      return String(raw)
    case 'N':
      return Number(raw)
    case 'BOOL':
      return raw === true
    case 'NULL':
      return null
    case 'B':
      return `b64:${String(raw)}`
    case 'SS':
    case 'BS':
      return Array.isArray(raw) ? raw.map(String) : []
    case 'NS':
      return Array.isArray(raw) ? raw.map(Number) : []
    case 'L':
      return Array.isArray(raw)
        ? raw.map((v) => plainAttr(v as DynamoAttr))
        : []
    case 'M':
      return plainMap(raw as DynamoItem)
    default:
      return null
  }
}

export function plainMap(item: DynamoItem): { [key: string]: PlainValue } {
  const out: { [key: string]: PlainValue } = {}
  for (const [k, v] of Object.entries(item)) out[k] = plainAttr(v)
  return out
}

// Short text for a grid cell; containers show as JSON.
export function cellText(attr: DynamoAttr | undefined): string {
  if (!attr) return ''
  const v = plainAttr(attr)
  return typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v)
}

export function attrType(attr: DynamoAttr | undefined): string {
  return attr ? (Object.keys(attr)[0] ?? '') : ''
}

// Key column names first, then the other attributes seen, in first-seen order.
export function gridColumns(items: DynamoItem[], keys: string[]): string[] {
  const seen = new Set<string>(keys)
  for (const it of items) for (const k of Object.keys(it)) seen.add(k)
  return [...seen]
}

// A typed key value from form text; numbers are validated, never coerced.
export function typedKey(type: string, text: string): DynamoAttr | null {
  if (text === '') return null
  if (type === 'N' && !/^-?\d+(\.\d+)?([eE][+-]?\d+)?$/.test(text)) return null
  return { [type === 'N' ? 'N' : type === 'B' ? 'B' : 'S']: text }
}
