import { parseEnvBlock, quoteValue } from './envParse'
import type { EnvTier } from './envProvenance'

// Must match internal/envredact.Placeholder: the server keeps the stored
// value whenever this exact string comes back for an existing key.
export const MASKED_VALUE = '••••••••'

export type EnvVarType = 'plain' | 'secret' | 'vault'

export interface EnvVarMeta {
  sensitive: boolean
  set: boolean
  value_redacted: boolean
  reason?: string
  override?: boolean
}

export interface VaultRef {
  path: string
  key: string
}

interface RowSnapshot {
  type: EnvVarType
  value: string
  vault?: VaultRef
}

export interface EnvRow {
  id: string
  key: string
  type: EnvVarType
  // Plain: the value or MASKED_VALUE. Secret: '' until replaced. Vault: ''.
  value: string
  vault?: VaultRef
  sensitive: boolean
  origin: 'own' | 'inherited'
  tier?: EnvTier
  original?: RowSnapshot
  removed?: boolean
}

export interface EnvRowSource {
  env?: Record<string, string>
  envMeta?: Record<string, EnvVarMeta>
  secretKeys: string[]
  vaultEnv?: Record<string, VaultRef>
  inherited: Array<{ key: string; value: string; tier: EnvTier }>
}

// Mirrors the server's name rule closely enough to suggest Secret on add.
const SENSITIVE_PARTS = [
  'KEY',
  'SECRET',
  'TOKEN',
  'PASSWORD',
  'PASSWD',
  'PRIVATE',
  'CREDENTIAL',
  'CREDENTIALS',
  'DSN',
  'AUTH',
  'SIGNING',
  'ENCRYPTION',
  'SALT',
  'APIKEY',
]
const PUBLIC_PREFIXES = [
  'NEXT_PUBLIC_',
  'VITE_',
  'REACT_APP_',
  'PUBLIC_',
  'NUXT_PUBLIC_',
  'EXPO_PUBLIC_',
  'GATSBY_',
]

export function looksSensitive(key: string): boolean {
  const upper = key.toUpperCase()
  if (PUBLIC_PREFIXES.some((p) => upper.startsWith(p))) {
    return false
  }
  if (
    ['PASSWORD', 'SECRET', 'TOKEN', 'DATABASE_URL', 'API_KEY'].some((s) =>
      upper.includes(s),
    )
  ) {
    return true
  }
  return upper
    .split(/[^A-Z0-9]+/)
    .some((part) => SENSITIVE_PARTS.includes(part))
}

let nextId = 0
export function newRowId(): string {
  nextId += 1
  return `row-${String(nextId)}`
}

export function buildRows(src: EnvRowSource): EnvRow[] {
  const rows: EnvRow[] = []
  const own = new Set<string>()
  for (const [key, value] of Object.entries(src.env ?? {})) {
    const meta = src.envMeta?.[key]
    own.add(key)
    rows.push({
      id: `own:${key}`,
      key,
      type: 'plain',
      value,
      sensitive: meta?.sensitive ?? value === MASKED_VALUE,
      origin: 'own',
      original: { type: 'plain', value },
    })
  }
  for (const key of new Set(src.secretKeys)) {
    if (own.has(key)) continue
    own.add(key)
    rows.push({
      id: `own:${key}`,
      key,
      type: 'secret',
      value: '',
      sensitive: true,
      origin: 'own',
      original: { type: 'secret', value: '' },
    })
  }
  for (const [key, ref] of Object.entries(src.vaultEnv ?? {})) {
    if (own.has(key)) continue
    own.add(key)
    rows.push({
      id: `own:${key}`,
      key,
      type: 'vault',
      value: '',
      vault: ref,
      sensitive: true,
      origin: 'own',
      original: { type: 'vault', value: '', vault: ref },
    })
  }
  for (const inh of src.inherited) {
    if (own.has(inh.key)) continue
    rows.push({
      id: `inherited:${inh.key}`,
      key: inh.key,
      type: 'plain',
      value: inh.value,
      sensitive: inh.value === MASKED_VALUE,
      origin: 'inherited',
      tier: inh.tier,
    })
  }
  return rows.sort((a, b) => a.key.localeCompare(b.key))
}

// An emptied Replace field on a masked value means "keep what is stored".
export function effectiveValue(row: EnvRow): string {
  if (
    row.type === 'plain' &&
    row.original?.value === MASKED_VALUE &&
    row.value === ''
  ) {
    return MASKED_VALUE
  }
  return row.value
}

export function isMasked(row: EnvRow): boolean {
  return row.type === 'plain' && row.value === MASKED_VALUE
}

export interface EnvChange {
  key: string
  type: EnvVarType
  kind: 'added' | 'changed' | 'removed'
  // Shown only for plain, non-sensitive values.
  from?: string
  to?: string
}

function visible(row: EnvRow, value: string | undefined): string | undefined {
  if (row.type !== 'plain' || row.sensitive || value === MASKED_VALUE) {
    return undefined
  }
  return value
}

export function computeChanges(rows: EnvRow[]): EnvChange[] {
  const out: EnvChange[] = []
  for (const row of rows) {
    if (row.origin !== 'own') continue
    const orig = row.original
    if (!orig) {
      if (!row.removed && row.key.trim() !== '') {
        out.push({
          key: row.key,
          type: row.type,
          kind: 'added',
          to: visible(row, row.value),
        })
      }
      continue
    }
    if (row.removed) {
      out.push({
        key: row.key,
        type: orig.type,
        kind: 'removed',
        from: visible(row, orig.value),
      })
      continue
    }
    const vaultChanged =
      row.type === 'vault' &&
      (row.vault?.path !== orig.vault?.path ||
        row.vault?.key !== orig.vault?.key)
    const valueChanged =
      row.type === 'secret'
        ? row.value !== ''
        : effectiveValue(row) !== orig.value
    if (
      row.type !== orig.type ||
      vaultChanged ||
      (row.type !== 'vault' && valueChanged)
    ) {
      out.push({
        key: row.key,
        type: row.type,
        kind: 'changed',
        from: visible(row, orig.value),
        to: visible(row, row.value),
      })
    }
  }
  return out.sort((a, b) => a.key.localeCompare(b.key))
}

export interface SavePlan {
  env: Record<string, string>
  envChanged: boolean
  secretsToSet: Array<{ key: string; value: string }>
  secretsToDelete: string[]
  vaultToSet: Array<{ key: string; ref: VaultRef }>
  vaultToClear: string[]
}

export function buildSavePlan(rows: EnvRow[]): SavePlan {
  const plan: SavePlan = {
    env: {},
    envChanged: false,
    secretsToSet: [],
    secretsToDelete: [],
    vaultToSet: [],
    vaultToClear: [],
  }
  for (const row of rows) {
    if (row.origin !== 'own' || row.key.trim() === '') continue
    const key = row.key.trim()
    const orig = row.original
    if (row.removed) {
      if (orig?.type === 'plain') plan.envChanged = true
      if (orig?.type === 'secret') plan.secretsToDelete.push(key)
      if (orig?.type === 'vault') plan.vaultToClear.push(key)
      continue
    }
    if (row.type === 'plain') {
      const value = effectiveValue(row)
      plan.env[key] = value
      if (!orig || orig.type !== 'plain' || orig.value !== value)
        plan.envChanged = true
    } else if (row.type === 'secret' && row.value !== '') {
      plan.secretsToSet.push({ key, value: row.value })
    } else if (row.type === 'vault' && row.vault) {
      const same =
        orig?.vault?.path === row.vault.path && orig.vault.key === row.vault.key
      if (!same) plan.vaultToSet.push({ key, ref: row.vault })
    }
  }
  return plan
}

// Bulk mode text: sensitive values stay masked unless revealed, so a pasted
// file can never expose them, and a masked line keeps the stored value.
export function rowsToDotenv(
  rows: EnvRow[],
  revealed: Record<string, string>,
): string {
  return rows
    .filter((r) => r.origin === 'own' && !r.removed && r.type !== 'vault')
    .map((r) => {
      const shown =
        revealed[r.key] ??
        (r.type === 'secret'
          ? r.value === ''
            ? MASKED_VALUE
            : r.value
          : r.value)
      return `${r.key}=${shown === MASKED_VALUE ? MASKED_VALUE : quoteValue(shown)}`
    })
    .join('\n')
}

export function applyDotenv(
  rows: EnvRow[],
  text: string,
  revealed: Record<string, string>,
): EnvRow[] {
  const parsed = new Map(parseEnvBlock(text).map((e) => [e.key, e.value]))
  const next = rows.map((row) => {
    if (row.origin !== 'own' || row.type === 'vault' || row.removed) return row
    if (!parsed.has(row.key)) return { ...row, removed: true }
    const incoming = parsed.get(row.key) ?? ''
    parsed.delete(row.key)
    if (incoming === MASKED_VALUE || incoming === revealed[row.key]) return row
    return { ...row, value: incoming }
  })
  for (const [key, value] of parsed) {
    const existing = next.find((r) => r.key === key && r.origin === 'inherited')
    if (value === MASKED_VALUE) continue
    next.push({
      id: newRowId(),
      key,
      type: 'plain',
      value,
      sensitive: looksSensitive(key),
      origin: 'own',
      tier: existing?.tier,
    })
  }
  return next.sort((a, b) => a.key.localeCompare(b.key))
}
