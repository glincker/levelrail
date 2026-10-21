import { describe, expect, it } from 'vitest'
import { certTruth } from './certTruth'
import type { CertificateStatus } from '../queries/certificates'

type C = Parameters<typeof certTruth>[0]

const base: Pick<CertificateStatus, 'source' | 'status'> = {
  source: 'acme',
  status: 'healthy',
}

describe('certTruth', () => {
  const cases: [string, C, Partial<ReturnType<typeof certTruth>>][] = [
    ['no certificate', undefined, { kind: 'none', wantsRealCert: true }],
    [
      'internal certificate warns and wants a real one',
      { ...base, kind: 'internal', days_left: 1 },
      { kind: 'internal', tone: 'warning', wantsRealCert: true },
    ],
    [
      'staging warns and wants a real one',
      { ...base, kind: 'staging', days_left: 80 },
      { kind: 'staging', tone: 'warning', wantsRealCert: true },
    ],
    [
      'lets encrypt far from expiry',
      { ...base, kind: 'letsencrypt', days_left: 60 },
      { kind: 'letsencrypt', tone: 'success', wantsRealCert: false },
    ],
    [
      'lets encrypt inside the failure window',
      { ...base, kind: 'letsencrypt', days_left: 9 },
      { tone: 'destructive' },
    ],
    [
      'failed renewal turns red',
      {
        ...base,
        kind: 'letsencrypt',
        days_left: 40,
        last_renewal: { at: '2026-10-01T00:00:00Z', result: 'failed' },
      },
      { tone: 'destructive', renewalFailed: true },
    ],
    [
      'expired is red whatever the kind',
      { ...base, kind: 'custom', days_left: -2 },
      { tone: 'destructive' },
    ],
    [
      'proxy is informational',
      { ...base, kind: 'proxy', days_left: 20 },
      { tone: 'info', wantsRealCert: false },
    ],
    [
      'older server without a kind: internal issuer',
      { ...base, issuer: 'Caddy Local Authority - ECC Intermediate' },
      { kind: 'internal', wantsRealCert: true },
    ],
  ]
  it.each(cases)('%s', (_name, cert, want) => {
    expect(certTruth(cert)).toMatchObject(want)
  })
})
