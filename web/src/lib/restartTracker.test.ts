import { describe, expect, it } from 'vitest'
import {
  nextRestartState,
  RESTART_TIMEOUT_MS,
  type RestartState,
} from './restartTracker'

const base: RestartState = {
  phase: 'restarting',
  oldBootId: 'old',
  startedAt: 1_000,
  sawDown: false,
}

describe('nextRestartState', () => {
  it('stays restarting while the old process still answers', () => {
    expect(nextRestartState(base, { ok: true, bootId: 'old' }, 2_000)).toBe(
      base,
    )
  })

  it('records that the server went down', () => {
    const s = nextRestartState(base, { ok: false }, 2_000)
    expect(s.sawDown).toBe(true)
    expect(s.phase).toBe('restarting')
  })

  it('completes only when the boot id changes', () => {
    const s = nextRestartState(base, { ok: true, bootId: 'new' }, 2_000)
    expect(s.phase).toBe('back')
  })

  it('times out when nothing changes', () => {
    const s = nextRestartState(
      base,
      { ok: false },
      base.startedAt + RESTART_TIMEOUT_MS + 1,
    )
    expect(s.phase).toBe('restarting')
    const t = nextRestartState(
      s,
      { ok: true, bootId: 'old' },
      base.startedAt + RESTART_TIMEOUT_MS + 1,
    )
    expect(t.phase).toBe('timeout')
  })
})
