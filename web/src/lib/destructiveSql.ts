export type DestructiveKind =
  | 'drop'
  | 'truncate'
  | 'deleteAll'
  | 'updateAll'
  | 'alterDrop'
  | 'grant'
  | 'revoke'

export interface DestructiveStatement {
  kind: DestructiveKind
  /** The table or object the statement names, empty when it cannot be read. */
  object: string
  /** DROP and TRUNCATE ask for the object name to be typed. */
  typeToConfirm: boolean
}

// Backticks quote identifiers on MySQL and MariaDB.
const PART = String.raw`(?:"[^"]+"|\x60[^\x60]+\x60|[\w$]+)`
const IDENT = String.raw`(${PART}(?:\.${PART})*)`

// Strings and comments can hold keywords, so blank them before matching.
function strip(sql: string): string {
  return sql
    .replace(/--[^\n]*/g, ' ')
    .replace(/\/\*[\s\S]*?\*\//g, ' ')
    .replace(/'(?:[^']|'')*'/g, "''")
}

function clean(name: string): string {
  return name.replace(/["`]/g, '')
}

// A leading WITH clause hides the real statement, so skip its balanced CTEs.
function stripCte(stmt: string): string {
  const head = /^with\s+(?:recursive\s+)?/i.exec(stmt)
  if (!head) return stmt
  let i = head[0].length
  for (;;) {
    const open = stmt.indexOf('(', i)
    if (open < 0) return stmt
    let depth = 0
    let j = open
    for (; j < stmt.length; j++) {
      if (stmt[j] === '(') depth++
      else if (stmt[j] === ')' && --depth === 0) break
    }
    if (depth !== 0) return stmt
    const rest = stmt.slice(j + 1)
    const comma = /^\s*,/.exec(rest)
    if (!comma) return rest.trim()
    i = j + 1 + comma[0].length
  }
}

function inspect(stmt: string): DestructiveStatement | null {
  const s = stripCte(stmt.trim())
  if (s === '') return null
  let m = new RegExp(
    String.raw`^drop\s+(?:table|view|schema|database|index|materialized\s+view|sequence|type|function|extension)\s+(?:if\s+exists\s+)?${IDENT}`,
    'i',
  ).exec(s)
  if (m) return { kind: 'drop', object: clean(m[1] ?? ''), typeToConfirm: true }
  m = new RegExp(
    String.raw`^truncate\s+(?:table\s+)?(?:only\s+)?${IDENT}`,
    'i',
  ).exec(s)
  if (m) {
    return { kind: 'truncate', object: clean(m[1] ?? ''), typeToConfirm: true }
  }
  m = new RegExp(
    String.raw`^alter\s+table\s+(?:if\s+exists\s+)?(?:only\s+)?${IDENT}`,
    'i',
  ).exec(s)
  if (m && /\bdrop\b/i.test(s.slice(m[0].length))) {
    return {
      kind: 'alterDrop',
      object: clean(m[1] ?? ''),
      typeToConfirm: false,
    }
  }
  m = new RegExp(String.raw`^delete\s+from\s+(?:only\s+)?${IDENT}`, 'i').exec(s)
  if (m && !/\bwhere\b/i.test(s.slice(m[0].length))) {
    return {
      kind: 'deleteAll',
      object: clean(m[1] ?? ''),
      typeToConfirm: false,
    }
  }
  m = new RegExp(String.raw`^update\s+(?:only\s+)?${IDENT}`, 'i').exec(s)
  if (m && !/\bwhere\b/i.test(s.slice(m[0].length))) {
    return {
      kind: 'updateAll',
      object: clean(m[1] ?? ''),
      typeToConfirm: false,
    }
  }
  if (/^grant\b/i.test(s)) {
    return {
      kind: 'grant',
      object: clean(/\bon\s+(?:table\s+)?(\S+)/i.exec(s)?.[1] ?? ''),
      typeToConfirm: false,
    }
  }
  if (/^revoke\b/i.test(s)) {
    return {
      kind: 'revoke',
      object: clean(/\bon\s+(?:table\s+)?(\S+)/i.exec(s)?.[1] ?? ''),
      typeToConfirm: false,
    }
  }
  return null
}

/** Every destructive statement in the SQL, in order. Read queries never match. */
export function detectAllDestructive(sql: string): DestructiveStatement[] {
  const hits: DestructiveStatement[] = []
  for (const part of strip(sql).split(';')) {
    const hit = inspect(part)
    if (hit) hits.push(hit)
  }
  return hits
}

/** Returns the first destructive statement in the SQL, or null. */
export function detectDestructive(sql: string): DestructiveStatement | null {
  return detectAllDestructive(sql)[0] ?? null
}

/** The name to type for a DROP or TRUNCATE: the last dotted part of the object. */
export function confirmName(d: DestructiveStatement): string {
  return d.object.split('.').pop() ?? d.object
}
