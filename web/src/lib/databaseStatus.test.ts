import { describe, expect, it } from 'vitest'
import type { ReconcileCondition } from '../types/deploy'
import { summarizeDatabaseStatus } from './databaseStatus'

function cond(
  Status: ReconcileCondition['Status'],
  Reason: string,
): ReconcileCondition {
  return {
    Type: 'Ready',
    Status,
    Reason,
    Message: '',
    LastTransitionTime: '2026-10-01T00:00:00Z',
  }
}

describe('summarizeDatabaseStatus', () => {
  it.each([
    [[], 'No status yet'],
    [[cond('Unknown', 'Suspended')], 'Stopped'],
    [
      [cond('True', 'AlreadyRunning'), cond('False', 'Crashed')],
      'Attention needed',
    ],
    [[cond('True', 'AlreadyRunning')], 'Healthy'],
    [
      [cond('True', 'AlreadyRunning'), cond('Unknown', 'NotConfigured')],
      'Healthy',
    ],
    [[cond('Unknown', 'Creating')], 'Reconciling'],
  ])('rolls %j up to %s', (conditions, label) => {
    expect(summarizeDatabaseStatus(conditions).label).toBe(label)
  })
})
