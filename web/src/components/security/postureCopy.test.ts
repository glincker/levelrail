import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { postureItemKey } from './postureCopy'

type Items = Record<string, Record<string, string> | undefined>

function loadItems(lng: string): Items {
  const raw = readFileSync(
    join(__dirname, '../../locales', lng, 'security.json'),
    'utf8',
  )
  return (JSON.parse(raw) as { items: Items }).items
}

const rules = readFileSync(
  join(__dirname, '../../../../internal/api/security_posture_rules.go'),
  'utf8',
)

// Rule ids appear as the first string literal of a rule constructor, a
// `const id =` or an `ID:` field in the Go rule list.
const ids = new Set<string>(
  [
    ...rules.matchAll(
      /(?:Rule\((?:in, )?|const id = |ID:\s*)"([a-z][a-z0-9]*(?:_[a-z0-9]+)+)"/g,
    ),
  ].map((m) => m[1] ?? ''),
)

describe('posture copy', () => {
  it('finds the backend rule ids', () => {
    expect(ids.size).toBeGreaterThan(10)
  })

  it('has en and es copy for every backend rule id', () => {
    const lookups: Items[] = [loadItems('en'), loadItems('es')]
    for (const id of ids) {
      expect(postureItemKey(id)).toBe(id)
      for (const items of lookups) {
        const item = items[id]
        expect(item?.title).toBeTruthy()
        expect(item?.pass).toBeTruthy()
        expect(item?.why).toBeTruthy()
      }
    }
  })

  it('covers the encrypted backup check', () => {
    expect(postureItemKey('offsite_backup_unencrypted')).not.toBeNull()
  })
})
