// Parses pasted or uploaded `.env` text into ordered key/value pairs.
// Handles comments, an `export ` prefix, single and double quotes, multiline
// quoted values, inline ` #` comments on unquoted values, BOM and CRLF.
// Duplicate keys are preserved in file order (last one wins when applied).
// No shell variable expansion. Mirrors cmd/levelrail-cli/env_dotenv.go.
//
// Shared by EnvVarsForm's paste dialog and EnvDevView, split out of the
// component files since a component file can only export components under
// this project's Fast Refresh lint rule.

export interface EnvEntry {
  key: string
  value: string
}

const KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_.-]*$/

function closingQuote(s: string, quote: string): number {
  for (let i = 0; i < s.length; i++) {
    if (quote === '"' && s[i] === '\\') {
      i++
      continue
    }
    if (s[i] === quote) return i
  }
  return -1
}

function unescapeDoubleQuoted(s: string): string {
  return s.replace(/\\([nrt"\\])/g, (_, c: string) => {
    switch (c) {
      case 'n':
        return '\n'
      case 'r':
        return '\r'
      case 't':
        return '\t'
      default:
        return c
    }
  })
}

function cutLine(s: string): [string, string] {
  const i = s.indexOf('\n')
  return i >= 0 ? [s.slice(0, i), s.slice(i + 1)] : [s, '']
}

function parseValue(v: string, rest: string): [string, string] {
  const quote = v[0]
  if (quote === '"' || quote === "'") {
    const full = `${v.slice(1)}\n${rest}`
    const end = closingQuote(full, quote)
    if (end >= 0) {
      const raw = full.slice(0, end)
      const [, remaining] = cutLine(full.slice(end + 1))
      return [quote === '"' ? unescapeDoubleQuoted(raw) : raw, remaining]
    }
  }
  let value = v
  for (const sep of [' #', '\t#']) {
    const i = value.indexOf(sep)
    if (i >= 0) value = value.slice(0, i)
  }
  return [value.trim(), rest]
}

export function parseEnvBlock(text: string): EnvEntry[] {
  let s = text.replace(/^\uFEFF/, '').replace(/\r\n/g, '\n')
  const results: EnvEntry[] = []
  while (s !== '') {
    let line: string
    ;[line, s] = cutLine(s)
    line = line.trim()
    if (!line || line.startsWith('#')) continue
    if (line.startsWith('export ')) line = line.slice('export '.length).trim()

    const eq = line.indexOf('=')
    if (eq === -1) continue
    const key = line.slice(0, eq).trim()
    if (!KEY_PATTERN.test(key)) continue

    let value: string
    ;[value, s] = parseValue(line.slice(eq + 1).trim(), s)
    results.push({ key, value })
  }
  return results
}

export type EnvImportStatus = 'new' | 'changed' | 'unchanged'

export interface EnvImportRow {
  key: string
  value: string
  status: EnvImportStatus
  // Set for a changed row: the value currently in the form.
  previous?: string
}

// Classifies parsed entries against the current rows. Later duplicates of
// a key win, and rows keep the file order of each key's first appearance.
export function classifyEnvImport(
  current: Record<string, string>,
  entries: EnvEntry[],
): EnvImportRow[] {
  const incoming = new Map<string, string>()
  for (const { key, value } of entries) incoming.set(key, value)
  return Array.from(incoming, ([key, value]) => {
    if (!(key in current)) return { key, value, status: 'new' as const }
    if (current[key] === value)
      return { key, value, status: 'unchanged' as const }
    return { key, value, status: 'changed' as const, previous: current[key] }
  })
}

export interface EnvChanges {
  added: string[]
  changed: string[]
  removed: string[]
}

// Diff of the form's pending variables against the saved ones.
export function diffEnv(
  saved: Record<string, string>,
  pending: Record<string, string>,
): EnvChanges {
  const added = Object.keys(pending).filter((k) => !(k in saved))
  const changed = Object.keys(pending).filter(
    (k) => k in saved && saved[k] !== pending[k],
  )
  const removed = Object.keys(saved).filter((k) => !(k in pending))
  return { added, changed, removed }
}

function quoteValue(v: string): string {
  if (v === '' || (v === v.trim() && !/["'#\n\r\\ \t]/.test(v))) return v
  const escaped = v
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/\n/g, '\\n')
    .replace(/\r/g, '\\r')
    .replace(/\t/g, '\\t')
  return `"${escaped}"`
}

// Builds `.env` text with sorted keys. Every secret key is written empty
// with a comment: secret values are never exported.
export function formatEnvExport(
  env: Record<string, string>,
  secretKeys: string[],
): string {
  const secrets = new Set(secretKeys)
  const keys = [
    ...Object.keys(env).filter((k) => !secrets.has(k)),
    ...secrets,
  ].sort()
  return keys
    .map((k) =>
      secrets.has(k)
        ? `# secret, value not exported\n${k}=\n`
        : `${k}=${quoteValue(env[k] ?? '')}\n`,
    )
    .join('')
}
