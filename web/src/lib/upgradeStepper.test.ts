import { describe, expect, it } from 'vitest'
import type { SelfUpgradeAttempt } from '../queries/selfUpgrade'
import { deriveStepper, type StepperInput } from './upgradeStepper'

const base: StepperInput = {
  updateAvailable: true,
  checkFailed: false,
  preflightBlocked: false,
}

function attempt(patch: Partial<SelfUpgradeAttempt>): SelfUpgradeAttempt {
  return {
    id: 'a',
    from_version: '1',
    to_version: '2',
    from_schema: 1,
    to_schema: 1,
    initiator: 'u',
    outcome: 'running',
    failed_step: '',
    error: '',
    backup_name: '',
    acked: [],
    steps: [],
    current_step: '',
    log: [],
    started_at: '2026-01-01T00:00:00Z',
    finished_at: '',
    ...patch,
  }
}

const statuses = (i: StepperInput) => deriveStepper(i).map((s) => s.status)

describe('deriveStepper', () => {
  it('up to date: only check is done', () => {
    expect(statuses({ ...base, updateAvailable: false })).toEqual([
      'done',
      'upcoming',
      'upcoming',
      'upcoming',
      'upcoming',
    ])
  })
  it('check failed', () => {
    expect(statuses({ ...base, checkFailed: true })[0]).toBe('failed')
  })
  it('preflight blocked', () => {
    expect(statuses({ ...base, preflightBlocked: true })).toEqual([
      'done',
      'blocked',
      'upcoming',
      'upcoming',
      'upcoming',
    ])
  })
  it('ready: download is next', () => {
    expect(statuses(base)).toEqual([
      'done',
      'done',
      'current',
      'upcoming',
      'upcoming',
    ])
  })
  it('running swap is the apply step', () => {
    expect(
      statuses({ ...base, attempt: attempt({ current_step: 'swap' }) }),
    ).toEqual(['done', 'done', 'done', 'current', 'upcoming'])
  })
  it('failed health check fails verify', () => {
    const a = attempt({ outcome: 'failed', failed_step: 'health' })
    expect(statuses({ ...base, attempt: a })).toEqual([
      'done',
      'done',
      'done',
      'done',
      'failed',
    ])
  })
  it('refused fails preflight', () => {
    const a = attempt({ outcome: 'refused' })
    expect(statuses({ ...base, attempt: a })[1]).toBe('failed')
  })
  it('succeeded is all done', () => {
    const a = attempt({ outcome: 'succeeded' })
    expect(statuses({ ...base, attempt: a }).every((s) => s === 'done')).toBe(
      true,
    )
  })
})
