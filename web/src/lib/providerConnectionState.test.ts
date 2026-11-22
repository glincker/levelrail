import { describe, expect, it } from 'vitest'
import { isProviderConnected, providerStatus } from './providerConnectionState'

describe('providerConnectionState', () => {
  it('is connected by an active account without a legacy token', () => {
    const r = {
      provider: 'oracle',
      has_token: false,
      connected: true,
      accounts: [],
    }
    expect(isProviderConnected(r)).toBe(true)
    expect(providerStatus(r)).toBe('connected')
  })
  it('falls back to the legacy token when connected is absent', () => {
    expect(isProviderConnected({ provider: 'aws', has_token: true })).toBe(true)
    expect(providerStatus({ provider: 'aws', has_token: false })).toBe(
      'not_connected',
    )
  })
  it('prefers a healthy account over a failed legacy test', () => {
    const acct = {
      id: 'a',
      label: 'a',
      status: 'active',
      auth_method: 'x',
      needs_relogin: false,
      managed: false,
    }
    const r = {
      provider: 'aws',
      has_token: true,
      connected: true,
      status: 'test_failed' as const,
      accounts: [acct],
    }
    expect(providerStatus(r)).toBe('connected')
    expect(providerStatus({ ...r, accounts: [] })).toBe('test_failed')
  })
})
