import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  CODE_KIND,
  ERROR_KINDS,
  STATUS_KIND,
  abilityFromMessage,
  isSafeServerText,
} from './errorCatalog'

interface Friendly {
  [kind: string]: { title?: string; message?: string; action?: string }
}

function friendly(lng: string): Friendly {
  const file = join(__dirname, '..', 'locales', lng, 'common.json')
  const json = JSON.parse(readFileSync(file, 'utf8')) as {
    apiError: { friendly: Friendly }
  }
  return json.apiError.friendly
}

describe('error catalog', () => {
  const kinds = new Set<string>(ERROR_KINDS)

  it('maps every code and status to a known kind', () => {
    for (const kind of [
      ...Object.values(CODE_KIND),
      ...Object.values(STATUS_KIND),
    ]) {
      expect(kinds.has(kind)).toBe(true)
    }
  })

  it.each(['en', 'es'])('has title, message and action in %s', (lng) => {
    const table = friendly(lng)
    for (const kind of ERROR_KINDS) {
      if (kind === 'generic') continue
      for (const part of ['title', 'message', 'action'] as const) {
        expect(table[kind]?.[part], `${lng} ${kind}.${part}`).toBeTruthy()
      }
    }
  })

  it('keeps es keys identical to en keys', () => {
    const keys = (t: Friendly) =>
      Object.entries(t)
        .flatMap(([k, v]) => Object.keys(v).map((p) => `${k}.${p}`))
        .sort()
    expect(keys(friendly('es'))).toEqual(keys(friendly('en')))
  })

  it('reads the missing ability from server text', () => {
    expect(
      abilityFromMessage('token lacks the required ability (needs write:apps)'),
    ).toBe('write:apps')
    expect(abilityFromMessage('which requires the root ability')).toBe('root')
    expect(abilityFromMessage('nope')).toBeUndefined()
  })

  it('rejects stack-like or oversized server text', () => {
    expect(isSafeServerText('name is required')).toBe(true)
    expect(isSafeServerText('panic: nil pointer')).toBe(false)
    expect(isSafeServerText('<html>bad gateway</html>')).toBe(false)
    expect(isSafeServerText('x'.repeat(500))).toBe(false)
    expect(isSafeServerText('internal error')).toBe(false)
  })
})
