import { describe, expect, it } from 'vitest'
import { shouldShowRestore } from './shouldShowRestore'

describe('shouldShowRestore', () => {
  it('needs a backup taken by the attempt', () => {
    const cmd = 'levelrail restore b.db'
    expect(
      shouldShowRestore({ outcome: 'succeeded', backup_name: 'b.db' }, cmd),
    ).toBe(true)
    expect(
      shouldShowRestore({ outcome: 'succeeded', backup_name: '' }, cmd),
    ).toBe(false)
    expect(shouldShowRestore(undefined, cmd)).toBe(false)
  })
})
