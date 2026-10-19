export const CHIP_KEYS = ['app', 'env', 'tag', 'node', 'kind'] as const
export type ChipKey = (typeof CHIP_KEYS)[number]

export interface Chip {
  key: ChipKey
  value: string
}

export interface ParsedInput {
  chips: Chip[]
  text: string
}

const CHIP_RE = /^(app|env|tag|node|kind):(\S+)$/i

export const KIND_VALUES = [
  'app',
  'database',
  'node',
  'domain',
  'template',
  'project',
  'environment',
  'tag',
  'deployment',
  'setting',
  'doc',
  'action',
] as const

export function chipFromToken(token: string): Chip | null {
  const m = CHIP_RE.exec(token)
  const key = m?.[1]
  const value = m?.[2]
  if (!key || !value) return null
  return { key: key.toLowerCase() as ChipKey, value }
}

// Finished `key:value ` tokens leave the text and become chips; the token
// still being typed (no trailing space yet) stays in the text.
export function parseInput(input: string): ParsedInput {
  const chips: Chip[] = []
  const endsWithSpace = /\s$/.test(input)
  const tokens = input.split(/\s+/).filter((t) => t !== '')
  const partial = endsWithSpace ? undefined : tokens.pop()
  const kept: string[] = []
  for (const token of tokens) {
    const chip = chipFromToken(token)
    if (chip) chips.push(chip)
    else kept.push(token)
  }
  let text = kept.join(' ')
  if (partial !== undefined) text = text === '' ? partial : `${text} ${partial}`
  else if (text !== '') text += ' '
  return { chips, text }
}

export function mergeChips(
  existing: readonly Chip[],
  added: readonly Chip[],
): Chip[] {
  const out = [...existing]
  for (const c of added) {
    if (!out.some((o) => o.key === c.key && o.value === c.value)) out.push(c)
  }
  return out
}

export function chipLabel(c: Chip): string {
  return `${c.key}:${c.value}`
}

export function buildServerQuery(chips: readonly Chip[], text: string): string {
  return [...chips.map(chipLabel), text.trim()]
    .filter((s) => s !== '')
    .join(' ')
}

export interface ScopeToken {
  key: ChipKey | null
  partialKey: string
  value: string
  start: number
}

// The trailing token of the input: either a partial scope key ("ap") or a
// scope value being typed ("app:fo").
export function trailingToken(text: string): ScopeToken | null {
  if (text === '' || /\s$/.test(text)) return null
  const start = text.lastIndexOf(' ') + 1
  const token = text.slice(start)
  const colon = token.indexOf(':')
  if (colon === -1) return { key: null, partialKey: token, value: '', start }
  const key = token.slice(0, colon).toLowerCase()
  if (!(CHIP_KEYS as readonly string[]).includes(key)) return null
  return {
    key: key as ChipKey,
    partialKey: key,
    value: token.slice(colon + 1),
    start,
  }
}

export function scopeKeyCompletions(partial: string): ChipKey[] {
  const p = partial.toLowerCase()
  if (p.length < 1) return []
  return CHIP_KEYS.filter((k) => k.startsWith(p) && k !== p)
}

export function freeTerms(text: string): string[] {
  return text
    .split(/\s+/)
    .filter((t) => t !== '' && !t.includes(':'))
    .map((t) => t.toLowerCase())
}
