import { describe, expect, it } from 'vitest'
import { ApiError, isDemoFinalError, shouldRetryQuery } from './apiError'

const demoErr = (code: string) => new ApiError(403, 'demo', { code })

describe('shouldRetryQuery', () => {
  it('does not retry demo refusals', () => {
    expect(shouldRetryQuery(0, demoErr('demo_readonly'))).toBe(false)
    expect(shouldRetryQuery(0, demoErr('demo_disabled'))).toBe(false)
    expect(isDemoFinalError(demoErr('demo_disabled'))).toBe(true)
  })
  it('keeps retrying network errors up to three times', () => {
    expect(shouldRetryQuery(0, new TypeError('Failed to fetch'))).toBe(true)
    expect(shouldRetryQuery(3, new TypeError('Failed to fetch'))).toBe(false)
  })
  it('retries an ordinary 403', () => {
    expect(
      shouldRetryQuery(0, new ApiError(403, 'nope', { code: 'forbidden' })),
    ).toBe(true)
  })
})
