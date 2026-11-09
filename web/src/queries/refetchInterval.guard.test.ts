import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

// Files allowed a literal poll interval. Empty: use pollMs tiers or named constants.
const LEGACY_LITERAL_POLLS = new Set<string>([])

const BARE_NUMBER = /(?:\?|:|=>|=)\s*\d{3,}\b/

function literalPollFiles(): string[] {
  const dir = __dirname
  const hits: string[] = []
  for (const name of readdirSync(dir)) {
    if (!name.endsWith('.ts') || name.endsWith('.test.ts')) continue
    const lines = readFileSync(join(dir, name), 'utf8').split('\n')
    const bare = lines.some((line, i) => {
      if (!/refetchInterval\s*:/.test(line)) return false
      const chunk = lines.slice(i, i + 3).join(' ')
      return BARE_NUMBER.test(chunk.replace(/^.*?refetchInterval/, ''))
    })
    if (bare) hits.push(name)
  }
  return hits.sort()
}

describe('refetchInterval guard', () => {
  it('uses pollMs tiers or named constants, not bare numbers', () => {
    const fresh = literalPollFiles().filter((f) => !LEGACY_LITERAL_POLLS.has(f))
    expect(
      fresh,
      `bare numeric refetchInterval in: ${fresh.join(', ')}`,
    ).toEqual([])
  })

  it('keeps the legacy list free of files that were fixed', () => {
    const current = new Set(literalPollFiles())
    const stale = [...LEGACY_LITERAL_POLLS].filter((f) => !current.has(f))
    expect(stale).toEqual([])
  })
})
