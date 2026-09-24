import { describe, expect, it } from 'vitest'
import { ApiError } from './apiError'
import {
  backoffDelayMs,
  isConnectivityError,
  recordFailure,
  safeReturnPath,
  shouldGoOffline,
} from './connectionState'

describe('backoffDelayMs', () => {
  it.each([
    [0, 2000],
    [1, 4000],
    [2, 8000],
    [3, 16000],
    [4, 30000],
    [50, 30000],
    [-3, 2000],
  ])('attempt %i waits %i ms', (attempt, want) => {
    expect(backoffDelayMs(attempt)).toBe(want)
  })
})

describe('isConnectivityError', () => {
  it.each([
    ['fetch network error', new TypeError('Failed to fetch'), true],
    ['502', new ApiError(502, 'bad gateway'), true],
    ['503', new ApiError(503, 'unavailable'), true],
    ['504', new ApiError(504, 'timeout'), true],
    ['401', new ApiError(401, 'no'), false],
    ['404', new ApiError(404, 'no'), false],
    ['500', new ApiError(500, 'boom'), false],
    ['plain error', new Error('x'), false],
    ['string', 'x', false],
  ])('%s', (_name, err, want) => {
    expect(isConnectivityError(err)).toBe(want)
  })
})

describe('offline threshold', () => {
  it('needs two failures inside the window', () => {
    let f = recordFailure([], 1000)
    expect(shouldGoOffline(f)).toBe(false)
    f = recordFailure(f, 5000)
    expect(shouldGoOffline(f)).toBe(true)
  })

  it('drops failures older than the window', () => {
    const f = recordFailure([1000], 20000)
    expect(f).toEqual([20000])
    expect(shouldGoOffline(f)).toBe(false)
  })
})

describe('safeReturnPath', () => {
  it.each([
    ['/apps/web', '/apps/web'],
    ['/apps?tab=logs', '/apps?tab=logs'],
    ['//evil.com', undefined],
    ['https://evil.com', undefined],
    ['/\\evil.com', undefined],
    ['/login', undefined],
    ['/login?setup=1', undefined],
    ['', undefined],
    [null, undefined],
    [42, undefined],
  ])('%j -> %j', (input, want) => {
    expect(safeReturnPath(input)).toBe(want)
  })
})
