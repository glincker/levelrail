import { describe, expect, it } from 'vitest'
import { deployCopyOf } from './deployCopy'

describe('deployCopyOf', () => {
  it('reads the target out of a RollbackTo reason', () => {
    expect(
      deployCopyOf({ reason: 'RollbackTo: dep_abcdef1234567890. by me' }),
    ).toEqual({ kind: 'rollback', target: 'dep_abcdef12' })
  })
  it('flags a promote without a target', () => {
    expect(deployCopyOf({ source: 'promote' })).toEqual({
      kind: 'promote',
      target: '',
    })
  })
  it('ignores a normal build', () => {
    expect(
      deployCopyOf({ source: 'webhook', reason: 'DetectedByPoll' }),
    ).toBeNull()
  })
})
