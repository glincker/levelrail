import type { DbSchemaNode, DbTable } from '../types/databaseViewer'

export type TreeItem =
  | { type: 'schema'; key: string; schema: string; count: number }
  | { type: 'table'; key: string; schema: string; table: DbTable }

export function tableKey(schema: string, table: string): string {
  return `${schema}.${table}`
}

function matches(schema: string, name: string, tokens: string[]): boolean {
  if (tokens.length === 0) return true
  const hay = `${schema}.${name}`.toLowerCase()
  return tokens.every((t) => hay.includes(t))
}

// A search term narrows to matching tables and expands every schema; with
// no term, collapsed schemas hide their tables. Output is a flat list so a
// virtualizer can window it.
export function buildTreeItems(
  schemas: DbSchemaNode[],
  query: string,
  collapsed: ReadonlySet<string>,
): TreeItem[] {
  const tokens = query.toLowerCase().split(/\s+/).filter(Boolean)
  const searching = tokens.length > 0
  const items: TreeItem[] = []
  for (const s of schemas) {
    const tables = s.tables.filter((tb) => matches(s.name, tb.name, tokens))
    if (tables.length === 0) continue
    items.push({
      type: 'schema',
      key: `schema:${s.name}`,
      schema: s.name,
      count: tables.length,
    })
    if (!searching && collapsed.has(s.name)) continue
    for (const tb of tables) {
      items.push({
        type: 'table',
        key: tableKey(s.name, tb.name),
        schema: s.name,
        table: tb,
      })
    }
  }
  return items
}

const STORAGE_PREFIX = 'explorer:last-table:'

export function readLastTable(database: string): string | null {
  try {
    return window.localStorage.getItem(STORAGE_PREFIX + database)
  } catch {
    return null
  }
}

export function writeLastTable(database: string, key: string): void {
  try {
    window.localStorage.setItem(STORAGE_PREFIX + database, key)
  } catch {
    // Storage can be blocked or full; the explorer works without it.
  }
}

export function formatRowEstimate(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`
  return `${(n / 1_000_000).toFixed(1)}M`
}
