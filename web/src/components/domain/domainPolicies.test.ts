import { describe, expect, it } from 'vitest'
import {
  PolicyApiError,
  fieldErrorsFor,
  toPolicyError,
} from '../../queries/domainPolicies'
import { diffFields } from './usePolicyDraft'
import { jsonResponse } from './testUtils'

describe('domainPolicies error mapping', () => {
  it('keeps per-field errors from a 400', async () => {
    const err = await toPolicyError(
      jsonResponse(
        {
          error: 'headers: rules[0].name: bad',
          fields: [{ field: 'rules[0].name', message: 'bad' }],
        },
        400,
      ),
      'fallback',
    )
    expect(err).toBeInstanceOf(PolicyApiError)
    expect(err.status).toBe(400)
    expect(err.message).toBe('headers: rules[0].name: bad')
    expect(fieldErrorsFor(err, 'headers')).toEqual({ 'rules[0].name': 'bad' })
  })

  it('strips the kind prefix and falls back when the body is not JSON', async () => {
    const prefixed = new PolicyApiError(400, 'x', [
      { field: 'cache.rules[1].ttl_seconds', message: 'too long' },
    ])
    expect(fieldErrorsFor(prefixed, 'cache')).toEqual({
      'rules[1].ttl_seconds': 'too long',
    })
    expect(fieldErrorsFor(new Error('x'), 'cache')).toEqual({})

    const bad = {
      ok: false,
      status: 502,
      json: () => Promise.reject(new Error('not json')),
    } as unknown as Response
    const err = await toPolicyError(bad, 'save failed: 502')
    expect(err.message).toBe('save failed: 502')
    expect(err.fields).toEqual([])
  })
})

describe('diffFields', () => {
  it('lists changed leaves only', () => {
    expect(
      diffFields(
        { rules: [{ name: 'a' }], hide_server: false },
        { rules: [{ name: 'b' }], hide_server: false },
      ),
    ).toEqual([{ path: 'rules.0.name', before: '"a"', after: '"b"' }])
  })
})
