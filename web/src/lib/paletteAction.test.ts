import { describe, expect, it } from 'vitest'
import { parseAppAction, parseDatabaseAction } from './paletteAction'

describe('parseAppAction', () => {
  it.each(['deploy', 'restart', 'stop', 'rollback'])('accepts %s', (a) => {
    expect(parseAppAction(a)).toBe(a)
  })
  it.each(['backup', 'delete', '', undefined, 42, null])('ignores %s', (a) => {
    expect(parseAppAction(a)).toBeUndefined()
  })
})

describe('parseDatabaseAction', () => {
  it('accepts backup only', () => {
    expect(parseDatabaseAction('backup')).toBe('backup')
    expect(parseDatabaseAction('deploy')).toBeUndefined()
  })
})
