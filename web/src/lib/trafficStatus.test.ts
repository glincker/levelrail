import { describe, expect, it } from 'vitest'
import type { CertificateStatus } from '../queries/certificates'
import type { AcmeAction } from '../queries/domainCheck'
import {
  DOMAIN_STATUSES,
  DOMAIN_STATUS_VARIANT,
  deriveDomainStatus,
  type DomainStatus,
  type DomainStatusInput,
} from './trafficStatus'

type Cert = NonNullable<DomainStatusInput['cert']>

function cert(over: Partial<CertificateStatus> = {}): Cert {
  return {
    status: 'healthy',
    renewal: 'ok',
    issuer: "Let's Encrypt",
    source: 'acme',
    ...over,
  }
}

const internal = cert({ issuer: 'Caddy Local Authority' })
const failure = (action: AcmeAction) => ({ action })

interface Case {
  name: string
  input: DomainStatusInput
  status: DomainStatus
  action: string
}

const cases: Case[] = [
  {
    name: 'row 1: automation run in progress beats everything',
    input: {
      runInProgress: true,
      maintenanceEnabled: true,
      check: 'connected',
    },
    status: 'going_live',
    action: 'view_progress',
  },
  {
    name: 'row 2: maintenance mode',
    input: { maintenanceEnabled: true, check: 'connected', cert: cert() },
    status: 'paused',
    action: 'turn_off',
  },
  {
    name: 'row 3: expired certificate',
    input: { check: 'connected', cert: cert({ status: 'expired' }) },
    status: 'needs_attention',
    action: 'fix',
  },
  {
    name: 'row 3: stalled renewal',
    input: { check: 'connected', cert: cert({ renewal: 'stalled' }) },
    status: 'needs_attention',
    action: 'fix',
  },
  ...(['open_port_80', 'fix_dns', 'fix_caa', 'check_logs'] as const).map(
    (action): Case => ({
      name: `row 3: acme failure ${action}`,
      input: { check: 'connected', acmeFailure: failure(action) },
      status: 'needs_attention',
      action: 'fix',
    }),
  ),
  {
    name: 'row 3: rate limit alone is not a failure to fix',
    input: {
      check: 'connected',
      acmeFailure: failure('wait_rate_limit'),
      cert: cert(),
    },
    status: 'live',
    action: 'open_site',
  },
  {
    name: 'row 3: failure carried on the certificate',
    input: {
      check: 'connected',
      cert: cert({
        acme_failure: {
          error: 'no such host',
          renewal: false,
          at: '2026-10-10T00:00:00Z',
          reason: 'dns',
          action: 'fix_dns',
        },
      }),
    },
    status: 'needs_attention',
    action: 'fix',
  },
  {
    name: 'row 3: resolves elsewhere in managed mode',
    input: { check: 'resolves_elsewhere', proxyMode: 'managed' },
    status: 'needs_attention',
    action: 'fix',
  },
  {
    name: 'row 3: resolves elsewhere is expected behind your own proxy',
    input: {
      check: 'resolves_elsewhere',
      proxyMode: 'external',
      routeManaged: false,
    },
    status: 'handled_by_proxy',
    action: 'show_proxy_setup',
  },
  {
    name: 'row 3: route error',
    input: { check: 'connected', routeState: 'error', cert: cert() },
    status: 'needs_attention',
    action: 'fix',
  },
  {
    name: 'row 4: external proxy, route not managed',
    input: { proxyMode: 'external', routeManaged: false, check: 'connected' },
    status: 'handled_by_proxy',
    action: 'show_proxy_setup',
  },
  {
    name: 'row 4: external proxy with a managed route continues',
    input: {
      proxyMode: 'external',
      routeManaged: true,
      check: 'connected',
      cert: cert(),
    },
    status: 'live',
    action: 'open_site',
  },
  {
    name: 'row 5: check unconfigured',
    input: { check: 'unconfigured' },
    status: 'not_set_up',
    action: 'set_up',
  },
  {
    name: 'row 5: no route',
    input: { check: 'connected', routeState: 'missing', cert: cert() },
    status: 'not_set_up',
    action: 'set_up',
  },
  {
    name: 'row 5: no check result yet',
    input: {},
    status: 'not_set_up',
    action: 'set_up',
  },
  {
    name: 'row 6: not resolving',
    input: { check: 'not_resolving' },
    status: 'waiting_for_dns',
    action: 'show_record',
  },
  {
    name: 'row 7: propagating',
    input: { check: 'propagating' },
    status: 'propagating',
    action: 'check_again',
  },
  {
    name: 'row 8: connected, first certificate pending',
    input: { check: 'connected', firstIssuancePending: true, cert: cert() },
    status: 'going_live',
    action: 'view_progress',
  },
  {
    name: 'row 8: connected, no certificate yet',
    input: { check: 'connected' },
    status: 'going_live',
    action: 'view_progress',
  },
  {
    name: 'row 9: expiring public certificate',
    input: { check: 'connected', cert: cert({ status: 'expiring_soon' }) },
    status: 'expiring_soon',
    action: 'renew',
  },
  {
    name: 'row 9: expiring custom certificate',
    input: {
      check: 'connected',
      cert: cert({ status: 'expiring_soon', source: 'custom' }),
    },
    status: 'expiring_soon',
    action: 'renew',
  },
  {
    name: 'row 9: internal CA expiry is never a warning',
    input: {
      check: 'connected',
      cert: { ...internal, status: 'expiring_soon' },
    },
    status: 'live',
    action: 'open_site',
  },
  {
    name: 'row 10: connected, healthy certificate',
    input: { check: 'connected', cert: cert() },
    status: 'live',
    action: 'open_site',
  },
  {
    name: 'row 10: private address on the internal CA',
    input: { check: 'connected', expectedPrivate: true, cert: internal },
    status: 'live',
    action: 'open_site',
  },
]

describe('deriveDomainStatus', () => {
  it.each(cases)('$name', ({ input, status, action }) => {
    const got = deriveDomainStatus(input)
    expect(got.status).toBe(status)
    expect(got.action).toBe(action)
  })

  it('uses the private reason for an internal CA on a private address', () => {
    const got = deriveDomainStatus({
      check: 'connected',
      expectedPrivate: true,
      cert: internal,
    })
    expect(got.reason).toBe('livePrivate')
  })

  it('reaches every status in the taxonomy', () => {
    const reached = new Set(cases.map((c) => c.status))
    expect([...reached].sort()).toEqual([...DOMAIN_STATUSES].sort())
  })

  it('maps every status to a badge variant', () => {
    for (const status of DOMAIN_STATUSES) {
      expect(DOMAIN_STATUS_VARIANT[status]).toBeTruthy()
    }
    expect(DOMAIN_STATUS_VARIANT.live).toBe('success')
    expect(DOMAIN_STATUS_VARIANT.needs_attention).toBe('destructive')
  })
})
