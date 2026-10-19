import { describe, expect, it } from 'vitest'
import { ApiError } from './apiError'
import { classifyPasskeyError, isRetryablePasskeyError } from './passkeyErrors'

function dom(name: string, message = ''): Error {
  const e = new Error(message)
  e.name = name
  return e
}

describe('classifyPasskeyError', () => {
  it.each([
    [dom('NotAllowedError', 'The operation was cancelled'), 'cancelled'],
    [dom('NotAllowedError', 'The operation timed out'), 'timeout'],
    [dom('InvalidStateError'), 'alreadyRegistered'],
    [dom('SecurityError'), 'insecureOrigin'],
    [dom('NotSupportedError'), 'unsupported'],
    [dom('PasskeyUnsupportedError'), 'unsupported'],
    [new ApiError(400, 'could not verify passkey'), 'rejected'],
    [new ApiError(409, 'only way in'), 'lastMethod'],
    [new ApiError(429, 'slow down'), 'rateLimited'],
    [new ApiError(501, 'off'), 'unavailable'],
    [new TypeError('Failed to fetch'), 'network'],
    [new Error('boom'), 'unknown'],
  ])('maps %# to %s', (error, kind) => {
    expect(classifyPasskeyError(error)).toBe(kind)
  })

  it('only offers retry where it can help', () => {
    expect(isRetryablePasskeyError('cancelled')).toBe(true)
    expect(isRetryablePasskeyError('alreadyRegistered')).toBe(false)
    expect(isRetryablePasskeyError('insecureOrigin')).toBe(false)
  })
})
