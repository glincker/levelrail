import { describe, expect, it } from 'vitest'
import jobsEn from '../locales/en/jobs.json'
import jobsEs from '../locales/es/jobs.json'
import type { ScheduledTask } from '../types/scheduledTasks'
import {
  guardsFromTask,
  guardsPatch,
  guardsSummaryParts,
  invalidGuardFields,
  overlapFromPolicy,
  policyFromOverlap,
} from './jobGuards'

const task: ScheduledTask = {
  id: 'j1',
  service_name: 'demo',
  command: ['sh', '-c', 'echo hi'],
  schedule: '0 3 * * *',
  enabled: true,
  concurrency_policy: 'forbid',
  overlap_policy: 'skip',
  timeout_seconds: 300,
  alert_on_failure: false,
  keep_runs: 0,
  keep_days: 0,
  consecutive_failures: 0,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
}

describe('overlap policy mapping', () => {
  it('maps the stored forbid to skip and back', () => {
    expect(overlapFromPolicy('forbid')).toBe('skip')
    expect(overlapFromPolicy('skip')).toBe('skip')
    expect(policyFromOverlap('skip')).toBe('forbid')
    expect(overlapFromPolicy('queue-one')).toBe('queue-one')
    expect(policyFromOverlap('queue-one')).toBe('queue-one')
    expect(overlapFromPolicy(undefined)).toBe('allow')
  })
})

describe('guardsPatch', () => {
  it('sends nothing when nothing changed', () => {
    const initial = guardsFromTask(task)
    expect(guardsPatch(initial, { ...initial })).toEqual({})
  })

  it('sends only the changed fields', () => {
    const initial = guardsFromTask(task)
    expect(
      guardsPatch(initial, {
        ...initial,
        overlap: 'queue-one',
        alertOnFailure: true,
        keepRuns: '50',
      }),
    ).toEqual({
      concurrency_policy: 'queue-one',
      alert_on_failure: true,
      keep_runs: 50,
    })
  })

  it('clears a timeout to the default with 0', () => {
    const initial = guardsFromTask(task)
    expect(guardsPatch(initial, { ...initial, timeoutSeconds: '' })).toEqual({
      timeout_seconds: 0,
    })
  })

  it('flags non whole numbers', () => {
    const initial = guardsFromTask(task)
    expect(
      invalidGuardFields({ ...initial, keepDays: '1.5', keepRuns: '-1' }),
    ).toEqual(['keepRuns', 'keepDays'])
  })
})

describe('guards summary and status labels', () => {
  it('lists only non default guards', () => {
    expect(guardsSummaryParts(task).map((p) => p.key)).toEqual([
      'overlap.skip',
      'timeout',
    ])
    expect(
      guardsSummaryParts({
        ...task,
        concurrency_policy: 'allow',
        overlap_policy: 'allow',
        timeout_seconds: 0,
      }),
    ).toEqual([])
  })

  it('has friendly labels for skipped and missed runs in en and es', () => {
    for (const locale of [jobsEn, jobsEs]) {
      expect(locale.status.skipped_concurrency).not.toMatch(/_/)
      expect(locale.status.missed.length).toBeGreaterThan(0)
      expect(locale.guards.overlap['queue-one'].length).toBeGreaterThan(0)
    }
  })
})
