import { afterEach, describe, expect, it, vi } from 'vitest'
import { applyDomainDns, planDomainDns } from './domainDns'

const rec = {
  name: 'shop.example.com',
  type: 'A',
  value: '203.0.113.7',
  ttl_seconds: 300,
}

function stub(handler: (url: string, body: unknown) => unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const body =
        typeof init?.body === 'string'
          ? (JSON.parse(init.body) as unknown)
          : undefined
      return Promise.resolve(
        new Response(JSON.stringify(handler(url, body)), { status: 200 }),
      )
    }),
  )
}
afterEach(() => vi.unstubAllGlobals())

describe('app domain dns via go-live', () => {
  it('plans through the go-live plan route', async () => {
    let seen = ''
    stub((url) => {
      seen = url
      return { plans: [{ dns: [{ dns: 'preview', record: rec }] }] }
    })
    const plan = await planDomainDns('shop.example.com', 'web')
    expect(seen).toBe('/api/v1/apps/web/domains/go-live/plan')
    expect(plan.hash).not.toBe('')
    expect(plan.changes[0]?.set.values).toEqual(['203.0.113.7'])
  })

  it('applies add-only through the go-live run route', async () => {
    let sent: unknown
    stub((url, body) => {
      sent = [url, body]
      return { dns: [{ dns: 'created', record: rec }] }
    })
    const res = await applyDomainDns('shop.example.com', 'go-live', 'web')
    expect(sent).toEqual([
      '/api/v1/apps/web/domains/shop.example.com/go-live',
      { dns: 'auto', replace: false },
    ])
    expect(res.applied).toBe(1)
  })
})

describe('server address fallback', () => {
  it('drops the alternative record so copied records do not conflict', async () => {
    stub(() => ({
      records: [
        { type: 'A', name: 'shop.example.com', value: '203.0.113.7' },
        {
          type: 'CNAME',
          name: 'shop.example.com',
          value: 'host.example.net',
          alternative: true,
        },
      ],
    }))
    const plan = await planDomainDns('shop.example.com')
    expect(plan.changes.map((c) => c.kind)).toEqual(['A'])
  })
})
