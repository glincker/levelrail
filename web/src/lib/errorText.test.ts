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
      'Something went wrong on the server.',
    )
  })
})
