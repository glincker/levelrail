import { afterEach, describe, expect, it, vi } from 'vitest'
import { importBundle } from './appBundle'

afterEach(() => vi.unstubAllGlobals())

describe('importBundle', () => {
  it('sends the file as the body with dry run and name in the query', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            app: 'shop',
            dry_run: true,
            sha256: 'a',
            services: [],
            deployed: false,
          }),
          { status: 200 },
        ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const blob = new Blob(['tar'])
    const res = await importBundle(blob, 'shop2', true)
    expect(res.app).toBe('shop')
    const [path, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(path).toBe('/api/v1/apps/import-bundle?name=shop2&dry_run=true')
    expect(init.body).toBe(blob)
  })

  it('throws the server error on a refusal', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: 'exists' }), { status: 409 }),
        ),
      ),
    )
    await expect(importBundle(new Blob(['x']), '', false)).rejects.toThrow()
  })
})
