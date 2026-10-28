import { describe, expect, it } from 'vitest'
import type { ReconcileCondition } from '../../types/deploy'
import { evaluatePower, type PowerTransition } from './appPower'

const timing = { graceMs: 1000, timeoutMs: 5000, pollMs: 100, failPolls: 3 }
const cond = (
  Status: ReconcileCondition['Status'],
  Reason = 'Ok',
): ReconcileCondition => ({
  Type: 'Ready',
  Status,
  Reason,
  Message: 'msg',
  LastTransitionTime: '',
})
const restart = (seenBusy = false): PowerTransition => ({
  kind: 'restart',
  startedAt: 0,
  seenBusy,
  slow: false,
  failStreak: 0,
  lastPollAt: 0,
})

describe('evaluatePower', () => {
  it('ignores a stale healthy status inside the grace window', () => {
    expect(evaluatePower(restart(), [cond('True')], 200, timing).state).toBe(
      'pending',
    )
  })

  it('finishes once healthy after the app was seen busy', () => {
    expect(
      evaluatePower(restart(true), [cond('True')], 200, timing).state,
    ).toBe('done')
  })

  it('finishes once healthy after the grace window', () => {
    expect(evaluatePower(restart(), [cond('True')], 1500, timing).state).toBe(
      'done',
    )
  })

  it('reports the failing condition as the reason', () => {
    const out = evaluatePower(
      restart(true),
      [cond('False', 'CrashLoop')],
      200,
      timing,
    )
    expect(out).toEqual({ state: 'failed', reason: 'CrashLoop: msg' })
  })

  it('treats a crash backoff as in progress, not a failure', () => {
    const out = evaluatePower(
      restart(true),
      [cond('False', 'CrashLoopBackOff')],
      200,
      timing,
    )
    expect(out.state).toBe('pending')
  })

  it('stop finishes when the app reports stopped', () => {
    const stop: PowerTransition = { ...restart(), kind: 'stop' }
    expect(
      evaluatePower(stop, [cond('False', 'Suspended')], 10, timing).state,
    ).toBe('done')
  })
})
