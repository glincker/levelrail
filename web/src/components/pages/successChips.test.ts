import { describe, expect, it } from 'vitest'
import { visibleSuccessChips } from './successChips'

describe('visibleSuccessChips', () => {
  it('shows all three chips to a writer', () => {
    const ids = visibleSuccessChips((a) => a === 'write').map((c) => c.id)
    expect(ids).toEqual(['domain', 'git', 'password'])
  })

  it('hides every chip for a read-only or unknown user', () => {
    expect(visibleSuccessChips((a) => a === 'read')).toEqual([])
    expect(visibleSuccessChips(() => false)).toEqual([])
  })
})
