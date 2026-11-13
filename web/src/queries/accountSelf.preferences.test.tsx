import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { clearStoredUsername, setStoredUsername } from '../lib/authStore'
import { type Preferences, useSavePreferences } from './accountSelf'

const START: Preferences = {
  timezone: 'UTC',
  language: 'en',
  theme: 'light',
  landing_page: '',
  gravatar: false,
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('useSavePreferences', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps both changes when two controls save at the same time', async () => {
    let server = { ...START }
    const bodies: Preferences[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_path: string, init?: RequestInit) => {
        if (init?.method !== 'PUT') return json(server)
        // The server answers slowly so the second save is issued while the first is in flight.
        await new Promise((r) => setTimeout(r, 20))
        const sent = JSON.parse(
          typeof init.body === 'string' ? init.body : '{}',
        ) as Preferences
        bodies.push(sent)
        server = sent
        return json(server)
      }),
    )
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useSavePreferences(), { wrapper })

    await act(async () => {
      await Promise.all([
        result.current.mutateAsync({ theme: 'dark' }),
        result.current.mutateAsync({ language: 'de' }),
      ])
    })

    await waitFor(() => {
      expect(bodies).toHaveLength(2)
    })
    expect(bodies[1]).toMatchObject({ theme: 'dark', language: 'de' })
    expect(server).toMatchObject({ theme: 'dark', language: 'de' })
  })

  it('drops a queued save when the signed-in account changed meanwhile', async () => {
    const puts: Preferences[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_path: string, init?: RequestInit) => {
        if (init?.method !== 'PUT') return json(START)
        await new Promise((r) => setTimeout(r, 20))
        const sent = JSON.parse(
          typeof init.body === 'string' ? init.body : '{}',
        ) as Preferences
        puts.push(sent)
        return json(sent)
      }),
    )
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useSavePreferences(), { wrapper })
    setStoredUsername('alice')

    let second: Promise<unknown> = Promise.resolve()
    await act(async () => {
      const first = result.current.mutateAsync({ theme: 'dark' })
      second = result.current
        .mutateAsync({ language: 'de' })
        .catch(() => 'dropped')
      // Switch accounts once the first save is in flight and the second is queued behind it.
      await new Promise((r) => setTimeout(r, 5))
      setStoredUsername('bob')
      await first
      await second
    })

    expect(puts).toHaveLength(1)
    expect(puts[0]).toMatchObject({ theme: 'dark' })
    await expect(second).resolves.toBe('dropped')
    clearStoredUsername()
  })
})
