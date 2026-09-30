import { describe, expect, it } from 'vitest'
import {
  CERT_RENEWAL_STALLED_HINT,
  certAttentionRank,
  certExpiryLabel,
  certRenewalBadge,
  sortByCertAttention,
} from './certStatus'
import type { CertificateStatus } from '../queries/certificates'

describe('certRenewalBadge', () => {
  it('flags a stalled renewal with the hint', () => {
    expect(certRenewalBadge({ renewal: 'stalled' })).toEqual({
      label: 'Renewal stalled',
      hint: CERT_RENEWAL_STALLED_HINT,
    })
  })
  it.each([['ok' as const], [undefined]])('is null for %s', (renewal) => {
    expect(certRenewalBadge({ renewal })).toBeNull()
  })
})

const now = new Date('2026-09-23T12:00:00Z')

describe('certExpiryLabel', () => {
  it.each([
    ['2026-10-23T12:00:00Z', 'expires in 30 days'],
    ['2026-09-24T18:00:00Z', 'expires in 1 day'],
    ['2026-09-23T20:00:00Z', 'expires today'],
    ['2026-09-20T12:00:00Z', 'expired 3 days ago'],
    ['2026-09-22T06:00:00Z', 'expired 1 day ago'],
    ['2026-09-23T06:00:00Z', 'expired today'],
    ['garbage', ''],
  ])('%s -> %s', (notAfter, want) => {
    expect(certExpiryLabel(notAfter, now)).toBe(want)
  })
})

function cert(
  overrides: Partial<CertificateStatus> & { domain: string },
): CertificateStatus {
  return {
    not_before: '2026-01-01T00:00:00Z',
    not_after: '2026-12-01T00:00:00Z',
    status: 'healthy',
    ...overrides,
  }
}

describe('certAttentionRank', () => {
  it('ranks stalled renewals first', () => {
    expect(
      certAttentionRank(
        cert({ domain: 'a', status: 'expired', renewal: 'stalled' }),
      ),
    ).toBe(0)
  })
  it('ranks other non-healthy statuses second', () => {
    expect(certAttentionRank(cert({ domain: 'a', status: 'expired' }))).toBe(1)
    expect(
      certAttentionRank(cert({ domain: 'a', status: 'expiring_soon' })),
    ).toBe(1)
  })
  it('ranks healthy or missing certs last', () => {
    expect(certAttentionRank(cert({ domain: 'a', status: 'healthy' }))).toBe(2)
    expect(certAttentionRank(undefined)).toBe(2)
  })
})

describe('sortByCertAttention', () => {
  it('puts stalled renewals first, then soonest expiry, then leaves the rest in order', () => {
    const certs: Record<string, CertificateStatus> = {
      healthy: cert({ domain: 'healthy', status: 'healthy' }),
      expiringLater: cert({
        domain: 'expiringLater',
        status: 'expiring_soon',
        not_after: '2026-10-20T00:00:00Z',
      }),
      expiringSoon: cert({
        domain: 'expiringSoon',
        status: 'expiring_soon',
        not_after: '2026-10-01T00:00:00Z',
      }),
      stalled: cert({
        domain: 'stalled',
        status: 'expired',
        renewal: 'stalled',
        not_after: '2026-09-01T00:00:00Z',
      }),
    }
    const domains = [
      'healthy',
      'expiringLater',
      'nocert',
      'stalled',
      'expiringSoon',
    ]
    const sorted = sortByCertAttention(domains, (d) => certs[d])
    expect(sorted).toEqual([
      'stalled',
      'expiringSoon',
      'expiringLater',
      'healthy',
      'nocert',
    ])
  })
})
