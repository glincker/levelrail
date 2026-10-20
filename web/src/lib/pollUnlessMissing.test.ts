import { describe, expect, it } from 'vitest'
import { ApiError } from './apiError'
import { pollUnlessMissing } from './pollUnlessMissing'

const q = (error: Error | null) => ({ state: { error } })

describe('pollUnlessMissing', () => {
  const poll = pollUnlessMissing(5000)
  it('keeps polling with no error or a transient error', () => {
    expect(poll(q(null))).toBe(5000)
    expect(poll(q(new ApiError(500, 'boom')))).toBe(5000)
  })
  it('stops on 404 and 501', () => {
    expect(poll(q(new ApiError(404, 'no')))).toBe(false)
    expect(poll(q(new ApiError(501, 'no')))).toBe(false)
  })
})
