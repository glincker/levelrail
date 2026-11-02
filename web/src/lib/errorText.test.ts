import { beforeAll, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { ApiError } from './apiError'
import { errorText } from './errorText'

describe('errorText', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('en')
    await i18n.loadNamespaces('common')
  })

  it('maps a 403 to a sentence with a next step', () => {
    const out = errorText(new ApiError(403, 'forbidden'))
    expect(out.message).toBe('Your account is not allowed to do this.')
    expect(out.action).toBeTruthy()
  })

  it('maps a failed fetch to a network sentence', () => {
    expect(errorText(new TypeError('Failed to fetch')).message).toBe(
      'Cannot reach the server.',
    )
  })

  it('keeps a specific validation message from the server', () => {
    expect(errorText(new ApiError(422, 'name is required')).message).toBe(
      'name is required',
    )
  })

  it('replaces the bare internal error text', () => {
    expect(errorText(new ApiError(500, 'internal error')).message).toBe(
      'The server hit an error. Try again.',
    )
  })

  it('maps coded errors to a title and a next step', () => {
    const out = errorText(new ApiError(404, 'x', { code: 'not_found' }))
    expect(out.title).toBe('Not found')
    expect(out.action).toBeTruthy()
  })

  it('names the missing ability on a 403', () => {
    const out = errorText(
      new ApiError(403, 'token lacks the required ability (needs write:apps)'),
    )
    expect(out.message).toContain('write:apps')
  })

  it('shows safe server text for an unknown 5xx with a retry hint', () => {
    const out = errorText(new ApiError(500, 'registry timed out'))
    expect(out.message).toBe('registry timed out')
    expect(out.action).toBeTruthy()
  })

  it('hides unsafe server text', () => {
    const out = errorText(new ApiError(418, 'panic: boom at main.go:12'))
    expect(out.message).toBe('The server hit an error. Try again.')
  })

  it('maps a timeout abort', () => {
    const out = errorText(new DOMException('t', 'TimeoutError'))
    expect(out.title).toBe('The request timed out')
  })
})
