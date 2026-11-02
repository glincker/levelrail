import { describe, expect, it } from 'vitest'
import { failureReason } from '../lib/failureReason'

describe('failureReason', () => {
  it('returns the failing condition message', () => {
    expect(
      failureReason({
        label: 'Attention needed',
        variant: 'destructive',
        reason: 'HealthCheckFailing',
        message: 'Health check never passes',
      }),
    ).toBe('Health check never passes')
  })

  it('falls back to the reason and ignores healthy rows', () => {
    expect(
      failureReason({
        label: 'Attention needed',
        variant: 'destructive',
        reason: 'CrashLoop',
      }),
    ).toBe('CrashLoop')
    expect(
      failureReason({ label: 'Healthy', variant: 'success', reason: 'x' }),
    ).toBeNull()
  })
})
