import { afterEach, describe, expect, it, vi } from 'vitest'
import { attentionCounts, fetchTrafficSummary } from './trafficSummary'

function respond(status: number, body: unknown = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify(body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    ),
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('fetchTrafficSummary', () => {
  it.each([404, 405, 501])('resolves null quietly on %i', async (status) => {
    respond(status)
    await expect(fetchTrafficSummary()).resolves.toBeNull()
  })

  it('returns the summary on success', async () => {
    respond(200, { domains: { attention: 2 } })
    await expect(fetchTrafficSummary()).resolves.toEqual({
      domains: { attention: 2 },
    })
  })

  it('throws on a real server error', async () => {
    respond(500, { error: 'boom' })
    await expect(fetchTrafficSummary()).rejects.toThrow()
  })
})

describe('attentionCounts', () => {
  it('reads each area', () => {
    expect(
      attentionCounts({
        domains: { attention: 3 },
        dns: { attention: 1 },
        proxy: { attention: 0 },
      }),
    ).toEqual({ domains: 3, dns: 1, proxy: 0 })
  })

  it('treats missing or bad values as zero', () => {
    expect(attentionCounts(null)).toEqual({ domains: 0, dns: 0, proxy: 0 })
    expect(attentionCounts({ domains: { attention: -4 }, dns: {} })).toEqual({
      domains: 0,
      dns: 0,
      proxy: 0,
    })
  })
})
