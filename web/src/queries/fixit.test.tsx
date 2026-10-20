import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFixForCheck, type Fix } from './fixit'

const addressFix = {
  id: 'mesh.public_address.set',
  check_id: 'public_address',
  failing: true,
} as Fix

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe('useFixForCheck', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('offers the address fix for the doctor advertise host check', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ fixes: [addressFix] }), {
            status: 200,
          }),
        ),
      ),
    )
    const { result } = renderHook(
      () => useFixForCheck('agent_advertise_reachability'),
      { wrapper },
    )
    await waitFor(() => {
      expect(result.current?.id).toBe('mesh.public_address.set')
    })
  })
})
