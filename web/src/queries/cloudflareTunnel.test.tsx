import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useCloudflareTunnelStatus } from './cloudflareTunnel'

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useCloudflareTunnelStatus', () => {
  it('sends no request while the feature is off', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    renderHook(() => useCloudflareTunnelStatus(false), { wrapper })
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('fetches the tunnel settings once the feature is on', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ enabled: false, status: 'disabled' }), {
          status: 200,
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderHook(() => useCloudflareTunnelStatus(true), { wrapper })
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
  })
})
