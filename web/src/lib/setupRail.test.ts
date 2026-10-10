import { describe, expect, it } from 'vitest'
import { railProgress, railState, setupNavReducer } from './setupRail'
import type { SetupStepId, SetupStepMap } from './setupWizard'

describe('railState', () => {
  const steps: SetupStepMap = {
    server: 'completed',
    topology: 'skipped',
    domain: 'completed',
  }
  const attention = new Set<SetupStepId>(['domain'])

  it('derives each state', () => {
    expect(railState('email', 'email', steps, attention)).toBe('current')
    expect(railState('server', 'email', steps, attention)).toBe('done')
    expect(railState('topology', 'email', steps, attention)).toBe('skipped')
    expect(railState('domain', 'email', steps, attention)).toBe('attention')
    expect(railState('git', 'email', steps, attention)).toBe('upcoming')
  })

  it('lets current win over attention', () => {
    expect(railState('domain', 'domain', steps, attention)).toBe('current')
  })
})

describe('railProgress', () => {
  it('counts completed and skipped as settled', () => {
    expect(railProgress({ server: 'completed', git: 'skipped' })).toEqual({
      done: 2,
      total: 7,
    })
  })
})

describe('setupNavReducer', () => {
  it('moves and clamps', () => {
    const at = (current: SetupStepId) => ({ current })
    expect(setupNavReducer(at('server'), { type: 'prev' }).current).toBe(
      'server',
    )
    expect(setupNavReducer(at('server'), { type: 'next' }).current).toBe(
      'topology',
    )
    expect(setupNavReducer(at('done'), { type: 'next' }).current).toBe('done')
    expect(
      setupNavReducer(at('server'), { type: 'goto', id: 'app' }).current,
    ).toBe('app')
  })
})
