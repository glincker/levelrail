import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './apiError'
import { apiRequest } from './apiRequest'

function mockFetch(res: Response) {
  const fn = vi.fn().mockResolvedValue(res)
  vi.stubGlobal('fetch', fn)
  return fn
}

describe('apiRequest', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('returns parsed JSON and sends no headers without a body', async () => {
    const fn = mockFetch(Response.json({ ok: true }))
    await expect(
      apiRequest('/x', { fallbackMessage: 'load x failed' }),
    ).resolves.toEqual({ ok: true })
    expect(fn).toHaveBeenCalledWith('/x', {
      method: undefined,
      headers: undefined,
      body: undefined,
    })
  })

  it('sends a JSON body with a content type', async () => {
    const fn = mockFetch(Response.json({}))
    await apiRequest('/x', {
      method: 'PUT',
      body: { a: 1 },
      fallbackMessage: 'f',
    })
    expect(fn).toHaveBeenCalledWith('/x', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: '{"a":1}',
    })
  })

  it('returns null on 204', async () => {
    mockFetch(new Response(null, { status: 204 }))
    await expect(apiRequest('/x', { fallbackMessage: 'f' })).resolves.toBeNull()
  })

  it('throws ApiError with the server message', async () => {
    mockFetch(Response.json({ error: 'nope' }, { status: 409 }))
    const err = await apiRequest('/x', { fallbackMessage: 'f' }).catch(
      (e: unknown) => e,
    )
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(409)
    expect((err as ApiError).message).toBe('nope')
  })

  it('falls back to the message plus status', async () => {
    mockFetch(new Response('<html>', { status: 502 }))
    await expect(
      apiRequest('/x', { fallbackMessage: 'load x failed' }),
    ).rejects.toThrow('load x failed: 502')
  })
})
