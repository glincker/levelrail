import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchAllZoneRecords } from './dns'

afterEach(() => vi.unstubAllGlobals())

describe('fetchAllZoneRecords', () => {
  it('follows next_cursor so large zones keep every record', async () => {
    const urls: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        urls.push(url)
        const second = url.includes('cursor=c2')
        const body = {
          records: [{ name: second ? 'b' : 'a', type: 'A', values: [] }],
          total: 2,
          next_cursor: second ? '' : 'c2',
        }
        return Promise.resolve(
          new Response(JSON.stringify(body), { status: 200 }),
        )
      }),
    )
    const out = await fetchAllZoneRecords('example.com')
    expect(out.records.map((r) => r.name)).toEqual(['a', 'b'])
    expect(urls).toHaveLength(2)
  })
})
