import { SUPPORTED_LANGUAGES } from './supported'

// Browser tags that name a supported language under another code. Regional Portuguese
// maps to the Brazilian files and any Simplified Chinese tag to zh-CN.
const ALIASES: Record<string, string> = {
  pt: 'pt-BR',
  'pt-pt': 'pt-BR',
  zh: 'zh-CN',
  'zh-hans': 'zh-CN',
  'zh-sg': 'zh-CN',
}

// Maps a detected browser or stored language tag onto a supported code, or returns it unchanged.
export function normalizeDetectedLanguage(raw: string): string {
  const tag = raw.replace('_', '-')
  const lower = tag.toLowerCase()
  const exact = SUPPORTED_LANGUAGES.find((c) => c.toLowerCase() === lower)
  if (exact) return exact
  const alias = ALIASES[lower]
  if (alias) return alias
  const parts = lower.split('-')
  // Traditional Chinese is not covered by zh-CN, so it falls through to English.
  if (
    parts[0] === 'zh' &&
    parts.some((p) => ['hant', 'tw', 'hk', 'mo'].includes(p))
  )
    return tag
  // Regional tags (es-MX, zh-Hans-CN) resolve to the longest supported or aliased prefix.
  for (let n = parts.length - 1; n >= 1; n--) {
    const head = parts.slice(0, n).join('-')
    const hit =
      ALIASES[head] ?? SUPPORTED_LANGUAGES.find((c) => c.toLowerCase() === head)
    if (hit) return hit
  }
  return tag
}
