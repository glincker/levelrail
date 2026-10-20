import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchAiProviders } from './aiProviders'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('fetchAiProviders', () => {
  it('returns the provider list from /providers', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify([{ id: 'openai' }]), { status: 200 }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    await expect(fetchAiProviders()).resolves.toEqual([{ id: 'openai' }])
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/settings/ai-assistant/providers',
    )
  })

  it('surfaces a missing endpoint as an error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('', { status: 404 }))),
    )
    await expect(fetchAiProviders()).rejects.toMatchObject({ status: 404 })
  })
})
