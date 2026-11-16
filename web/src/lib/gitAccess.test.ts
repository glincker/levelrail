import { describe, expect, it } from 'vitest'
import {
  capabilityState,
  daysUntil,
  GIT_ACCESS_METHODS,
  parseScopes,
} from './gitAccess'

describe('gitAccess', () => {
  it('parses scopes split by space or comma, null when none', () => {
    expect(parseScopes('api read_user')).toEqual(['api', 'read_user'])
    expect(parseScopes('a,b')).toEqual(['a', 'b'])
    expect(parseScopes('')).toBeNull()
    expect(parseScopes(undefined)).toBeNull()
  })

  it('derives capability state from granted scopes', () => {
    expect(capabilityState('gitlab', 'register_webhook', ['api'])).toEqual({
      kind: 'allowed',
    })
    expect(capabilityState('gitlab', 'register_webhook', ['read_api'])).toEqual(
      {
        kind: 'needs_scope',
        scope: 'api',
      },
    )
    expect(capabilityState('gitlab', 'registry_pull', ['api'])).toEqual({
      kind: 'needs_scope',
      scope: 'read_registry',
    })
    expect(capabilityState('gitlab', 'register_webhook', null)).toEqual({
      kind: 'unknown',
    })
    expect(capabilityState('bitbucket_server', 'mr_comments', null)).toEqual({
      kind: 'not_allowed',
    })
    expect(capabilityState('azure_devops', 'register_webhook', ['x'])).toEqual({
      kind: 'unknown',
    })
  })

  it('counts days to expiry and ignores bad dates', () => {
    const now = Date.parse('2026-11-15T00:00:00Z')
    expect(daysUntil('2026-11-20T00:00:00Z', now)).toBe(5)
    expect(daysUntil(null, now)).toBeNull()
    expect(daysUntil('nope', now)).toBeNull()
  })

  it('offers deploy and CI methods only where a page exists for them', () => {
    expect(GIT_ACCESS_METHODS.gitlab).toContain('deploy_token')
    expect(GIT_ACCESS_METHODS.gitlab).toContain('ci_job')
    expect(GIT_ACCESS_METHODS.azure_devops).toEqual(['token'])
  })
})
