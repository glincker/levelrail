import { describe, expect, it } from 'vitest'
import { activeCliTokens, cliTokenStatus } from './cliConnection'

const now = Date.parse('2026-06-01T12:00:00Z')
const future = '2026-07-01T00:00:00Z'
const past = '2026-05-01T00:00:00Z'

describe('activeCliTokens', () => {
  it('keeps a live cli-pairing token', () => {
    const tokens = [{ name: 'cli-pairing', expires_at: future }]
    expect(activeCliTokens(tokens, now)).toHaveLength(1)
  })

  it('keeps a prefix followed by a client name', () => {
    const tokens = [{ name: 'cli-pairing laptop' }]
    expect(activeCliTokens(tokens, now)).toHaveLength(1)
  })

  it('drops revoked tokens', () => {
    const tokens = [{ name: 'cli-pairing', revoked_at: past }]
    expect(activeCliTokens(tokens, now)).toHaveLength(0)
  })

  it('drops expired tokens', () => {
    const tokens = [{ name: 'cli-pairing', expires_at: past }]
    expect(activeCliTokens(tokens, now)).toHaveLength(0)
  })

  it('drops tokens with other names', () => {
    const tokens = [{ name: 'ci-deploy' }, { name: 'my-cli-pairing' }]
    expect(activeCliTokens(tokens, now)).toHaveLength(0)
  })

  it('treats a token with no expiry as live', () => {
    const tokens = [{ name: 'cli-pairing', expires_at: null }]
    expect(activeCliTokens(tokens, now)).toHaveLength(1)
  })
})

describe('cli origins and status', () => {
  it('keeps device-flow and password-login tokens', () => {
    const tokens = [
      { name: 'device: Mac.lan' },
      { name: 'levelrail-cli-20260101-000000' },
      { name: 'github-actions' },
    ]
    expect(activeCliTokens(tokens, now)).toHaveLength(2)
  })

  it('reports expiring within the window and expired after', () => {
    expect(
      cliTokenStatus({ name: 'a', expires_at: '2026-06-03T00:00:00Z' }, now),
    ).toBe('expiring')
    expect(cliTokenStatus({ name: 'a', expires_at: future }, now)).toBe(
      'active',
    )
    expect(cliTokenStatus({ name: 'a', expires_at: past }, now)).toBe('expired')
  })
})
