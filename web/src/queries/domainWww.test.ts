import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { domainWwwPlanQueryOptions } from './domainWww'

afterEach(() => vi.unstubAllGlobals())

describe('domainWwwPlanQueryOptions', () => {
  it('sends mode and canonical and returns the plan', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        Response.json({ domain: 'a.com', records: [], redirect: null }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const qc = new QueryClient()
    const plan = await qc.fetchQuery(
      domainWwwPlanQueryOptions('app', 'a.com', {
        mode: 'both',
        canonical: 'www',
      }),
    )
    expect(plan.domain).toBe('a.com')
    const url = String((fetchMock.mock.calls[0] as unknown[])[0])
    expect(url).toContain('/apps/app/domains/a.com/dns/plan?')
    expect(url).toContain('canonical=www')
  })

  it('throws an ApiError carrying the server message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(new Response('{"error":"nope"}', { status: 400 })),
      ),
    )
    await expect(
      new QueryClient().fetchQuery(
        domainWwwPlanQueryOptions('app', 'a.com', {
          mode: 'both',
          canonical: 'apex',
        }),
      ),
    ).rejects.toMatchObject({ status: 400 })
  })
})
