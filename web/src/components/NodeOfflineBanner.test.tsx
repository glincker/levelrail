import { describe, expect, it } from 'vitest'
import { summarizeAppStatus, isNodeOfflineStatus } from '../lib/appStatus'
import type { ReconcileCondition } from '../types/deploy'

const offline: ReconcileCondition = {
  Type: 'Ready',
  Status: 'False',
  Reason: 'NodeOffline',
  Message: 'node worker-b is offline for 53s',
  LastTransitionTime: '2026-01-01T00:00:00Z',
}

describe('node offline status', () => {
  it('detects the NodeOffline condition', () => {
    expect(isNodeOfflineStatus([offline])).toBe(true)
    expect(isNodeOfflineStatus([{ ...offline, Reason: 'CreateFailed' }])).toBe(
      false,
    )
  })

  it('labels the app badge as node offline instead of generic attention', () => {
    expect(summarizeAppStatus([offline])).toEqual({
      label: 'Node offline',
      variant: 'destructive',
    })
  })
})
