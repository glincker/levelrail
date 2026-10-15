import { describe, expect, it } from 'vitest'
import { resolveMethod, supportedMethods } from './nodeProviderMethods'

describe('nodeProviderMethods', () => {
  const aws = supportedMethods('aws', undefined)

  it('accepts short aliases the provider supports', () => {
    expect(resolveMethod('role', aws)).toBe('assume_role')
    expect(resolveMethod('assume_role', aws)).toBe('assume_role')
  })

  it('falls back to the stored then the first supported method', () => {
    expect(resolveMethod('nope', aws, 'ambient')).toBe('ambient')
    expect(resolveMethod(undefined, aws)).toBe('access_key')
    expect(resolveMethod('json', ['api_token'])).toBe('api_token')
  })

  it('prefers the API list over the built-in fallback', () => {
    expect(supportedMethods('hetzner', ['manual'])).toEqual(['manual'])
  })
})
