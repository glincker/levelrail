import { fireEvent, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { IngressFallbackCertsCard } from './IngressFallbackCertsCard'
import { renderWithQueryI18n } from '@/test/renderWithQuery'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const setting = {
  key: 'ingress.fallback_certs',
  value: 'lazy',
  default: 'eager',
  source: 'saved',
  env: 'APP_INGRESS_FALLBACK_CERTS',
  allowed_strings: ['eager', 'lazy'],
}

function json(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
    clone: () => ({ json: () => Promise.resolve(body) }),
  } as unknown as Response
}

describe('IngressFallbackCertsCard', () => {
  it('shows the source and resets to the default', async () => {
    const fn = vi.fn((_i: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'DELETE')
        return Promise.resolve(json({ ...setting, value: 'eager' }))
      return Promise.resolve(
        json({ settings: [setting], restart: { supported: false } }),
      )
    })
    vi.stubGlobal('fetch', fn)
    renderWithQueryI18n(<IngressFallbackCertsCard />)
    expect(await screen.findByText('Saved')).toBeTruthy()
    expect(
      screen.getByText(/first visit to the automatic address/),
    ).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Reset to default' }))
    await waitFor(() => {
      const del = fn.mock.calls.find(([, i]) => i?.method === 'DELETE')
      expect(del?.[0] as string).toContain('ingress.fallback_certs')
    })
  })
})
