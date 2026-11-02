import { fireEvent, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { IngressCloudflareSwitch } from './IngressCloudflareSwitch'
import type { IngressCloudflare } from '../queries/ingressCloudflare'
import { renderWithQueryI18n } from '@/test/renderWithQuery'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const base: IngressCloudflare = {
  preset: 'none',
  source: 'default',
  ranges: 22,
  ranges_source: 'embedded',
  refresh_interval: '24h0m0s',
}

function json(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

describe('IngressCloudflareSwitch', () => {
  it('turning it on sends the preset with the weakening confirmation', async () => {
    const fn = vi.fn((_i: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'PUT')
        return Promise.resolve(json({ ...base, preset: 'cloudflare' }))
      return Promise.resolve(json(base))
    })
    vi.stubGlobal('fetch', fn)
    renderWithQueryI18n(<IngressCloudflareSwitch />)
    fireEvent.click(await screen.findByRole('switch'))
    await waitFor(() => {
      const put = fn.mock.calls.find(([, i]) => i?.method === 'PUT')
      expect(put?.[1]?.body).toBe(
        JSON.stringify({ preset: 'cloudflare', confirm_weakening: true }),
      )
    })
  })
})
