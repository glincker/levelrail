import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  fetchDnsStatus,
  progressOf,
  statusFor,
  type DnsStatusReport,
} from './dnsStatus'
import { applyDomainDns, planDomainDns } from './domainDns'
import { startDomainConnectApply } from './domainConnect'

const ok = (body: unknown, status = 200) =>
  Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )

const report: DnsStatusReport = {
  provider_source: 'provider',
  checked_at: '2026-10-30T00:00:00Z',
  counts: {
    live: 1,
    waiting: 1,
    different: 0,
    conflict: 0,
    unknown: 0,
    total: 2,
  },
  records: [
    {
      id: 'mx',
      type: 'MX',
      name: 'example.com',
      value: 'mail.example.com',
      ttl: 300,
      priority: 10,
      status: 'live',
      found: [{ type: 'MX', name: 'example.com', value: 'mail.example.com' }],
      checked_at: '2026-10-30T00:00:00Z',
    },
  ],
}

afterEach(() => vi.unstubAllGlobals())

describe('fetchDnsStatus', () => {
  it('calls the mail route with the plan spec fields', async () => {
    const f = vi.fn(() => ok(report))
    vi.stubGlobal('fetch', f)
    const out = await fetchDnsStatus('mail', 'example.com', undefined, {
      domain: 'example.com',
      mail_host: 'mail.example.com',
      ipv4: '203.0.113.7',
    })
    expect(out?.records[0]?.status).toBe('live')
    const url = String((f.mock.calls[0] as unknown[])[0])
    expect(url).toContain('/api/v1/mail/dns/status?')
    expect(url).toContain('mail_host=mail.example.com')
    expect(url).toContain('ipv4=203.0.113.7')
  })
  it('calls the app route with domain and app', async () => {
    const f = vi.fn(() => ok(report))
    vi.stubGlobal('fetch', f)
    await fetchDnsStatus('app', 'app.example.com', 'web')
    const url = String((f.mock.calls[0] as unknown[])[0])
    expect(url).toContain('/api/v1/domains/dns-status?')
    expect(url).toContain('app=web')
  })
  it('falls back to null on 404 and on network failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => ok({}, 404)),
    )
    expect(await fetchDnsStatus('mail', 'example.com')).toBeNull()
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('down'))),
    )
    expect(await fetchDnsStatus('app', 'example.com')).toBeNull()
  })
})

describe('status helpers', () => {
  it('matches records ignoring case and trailing dots', () => {
    expect(statusFor(report, 'EXAMPLE.com.', 'mx')?.id).toBe('mx')
    expect(statusFor(report, 'other.com', 'MX')).toBeUndefined()
  })
  it('header counts use the server tally, else the rows', () => {
    expect(progressOf(['waiting'], report)).toEqual({ live: 1, total: 2 })
    expect(progressOf(['live', 'waiting', 'live'], null)).toEqual({
      live: 2,
      total: 3,
    })
  })
})

describe('go-live plan hash', () => {
  const planBody = {
    plans: [
      {
        plan_hash: 'abc123',
        dns: [
          {
            dns: 'preview',
            record: {
              name: 'app.example.com',
              type: 'A',
              value: '1.2.3.4',
              ttl_seconds: 300,
            },
          },
        ],
      },
    ],
  }
  it('pins the plan hash and sends it back on apply', async () => {
    const bodies: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.endsWith('/go-live/plan')) return ok(planBody)
        bodies.push(init?.body as string)
        return ok({ dns: [] })
      }),
    )
    const plan = await planDomainDns('app.example.com', 'web')
    expect(plan.hash).toBe('abc123')
    await applyDomainDns('app.example.com', plan.hash, 'web')
    expect(JSON.parse(bodies[0] ?? '{}')).toMatchObject({ plan_hash: 'abc123' })
  })
  it('stays unpinned against a server without plan_hash', async () => {
    const bodies: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.endsWith('/go-live/plan'))
          return ok({ plans: [{ dns: planBody.plans[0]?.dns }] })
        bodies.push(init?.body as string)
        return ok({ dns: [] })
      }),
    )
    const plan = await planDomainDns('app.example.com', 'web')
    await applyDomainDns('app.example.com', plan.hash, 'web')
    expect(JSON.parse(bodies[0] ?? '{}')).not.toHaveProperty('plan_hash')
  })
})

describe('startDomainConnectApply', () => {
  it('posts the domain and opens the returned url in a new tab', async () => {
    const f = vi.fn(() => ok({ url: 'https://dns.example.net/approve?x=1' }))
    const open = vi.fn()
    vi.stubGlobal('fetch', f)
    vi.stubGlobal('open', open)
    const url = await startDomainConnectApply('example.com', 'mail')
    expect(url).toBe('https://dns.example.net/approve?x=1')
    const call = f.mock.calls[0] as unknown as [string, RequestInit]
    expect(call[0]).toBe('/api/v1/domains/domain-connect/start')
    expect(JSON.parse(call[1].body as string)).toEqual({
      domain: 'example.com',
      kind: 'mail',
    })
    expect(open).toHaveBeenCalledWith(url, '_blank', 'noopener,noreferrer')
  })
  it('rejects a response without a usable https url', async () => {
    const open = vi.fn()
    vi.stubGlobal(
      'fetch',
      vi.fn(() => ok({ url: 'javascript:alert(1)' })),
    )
    vi.stubGlobal('open', open)
    await expect(
      startDomainConnectApply('example.com', 'app'),
    ).rejects.toThrow()
    expect(open).not.toHaveBeenCalled()
  })
})
