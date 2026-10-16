import { describe, expect, it } from 'vitest'
import type {
  SelfUpgradeAttempt,
  SelfUpgradeStep,
} from '../queries/selfUpgrade'
import { buildPipeline, phaseOf } from './upgradePhases'

const planned = [
  'download',
  'verify_checksum',
  'verify_signature',
  'probe_binary',
  'backup',
  'migration_check',
  'stop',
  'swap',
  'start',
  'health',
]

const step = (
  name: string,
  status: SelfUpgradeStep['status'] = 'ok',
): SelfUpgradeStep => ({
  name,
  status,
  detail: '',
  at: '2026-10-15T10:00:10Z',
  duration_ms: 2000,
})

function attempt(over: Partial<SelfUpgradeAttempt>): SelfUpgradeAttempt {
  return {
    id: 'a',
    from_version: 'v1',
    to_version: 'v2',
    from_schema: 1,
    to_schema: 1,
    initiator: '',
    outcome: 'running',
    failed_step: '',
    error: '',
    backup_name: '',
    acked: [],
    steps: [],
    current_step: '',
    log: [],
    started_at: '2026-10-15T10:00:00Z',
    finished_at: '',
    ...over,
  }
}

describe('phaseOf', () => {
  it.each([
    ['download', null, 'prepare'],
    ['backup', null, 'protect'],
    ['health', null, 'confirm'],
    ['weird', 'switch', 'switch'],
    ['weird', null, 'other'],
  ])('%s after %s is %s', (name, prev, want) => {
    expect(phaseOf(name, prev as never)).toBe(want)
  })
})

describe('buildPipeline', () => {
  it('handles an empty attempt with default steps and unknown percent', () => {
    const v = buildPipeline([], undefined)
    expect(v.percent).toBeNull()
    expect(v.phases.map((p) => p.id)).toEqual([
      'prepare',
      'validate',
      'protect',
      'switch',
      'confirm',
    ])
    expect(v.phases.every((p) => p.state === 'pending')).toBe(true)
  })

  it('computes percent and running phase mid-run', () => {
    const v = buildPipeline(
      planned,
      attempt({
        steps: [step('guard'), step('download'), step('verify_checksum')],
        current_step: 'verify_signature',
      }),
    )
    expect(v.currentStep).toBe('verify_signature')
    expect(v.currentPhase).toBe('prepare')
    expect(v.phases[0]?.state).toBe('running')
    expect(v.percent).toBe(27)
    expect(v.stepIndex).toBe(4)
    expect(v.total).toBe(11)
  })

  it('marks the phase failed mid-phase', () => {
    const v = buildPipeline(
      planned,
      attempt({
        outcome: 'failed',
        steps: [
          step('guard'),
          step('download'),
          step('verify_checksum', 'failed'),
        ],
      }),
    )
    expect(v.phases[0]?.state).toBe('failed')
    expect(v.phases[1]?.state).toBe('pending')
  })

  it('groups an unknown step with the previous phase and flags rollback', () => {
    const v = buildPipeline(
      planned,
      attempt({
        outcome: 'rolled_back',
        steps: [step('guard'), step('stop'), step('mystery'), step('rollback')],
      }),
    )
    const sw = v.phases.find((p) => p.id === 'switch')
    expect(v.rolledBack).toBe(true)
    expect(v.phases.some((p) => p.id === 'other')).toBe(false)
    expect(sw?.steps.map((s) => s.name)).not.toContain('mystery')
  })

  it('reports 100 when finished', () => {
    const v = buildPipeline(
      planned,
      attempt({
        outcome: 'succeeded',
        steps: ['guard', ...planned].map((n) => step(n)),
      }),
    )
    expect(v.percent).toBe(100)
    expect(v.phases.every((p) => p.state === 'ok')).toBe(true)
  })
})
