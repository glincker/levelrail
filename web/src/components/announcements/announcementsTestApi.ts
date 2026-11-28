import { vi } from 'vitest'
import type { Announcement } from '@/queries/announcements'

export interface Call {
  url: string
  method: string
  body: string | undefined
}

export function ann(over: Partial<Announcement> = {}): Announcement {
  return {
    id: 'a1',
    tier: 'advisory',
    title: 'Patch available',
    body: 'First paragraph.',
    links: null,
    issued_at: '2026-11-01T00:00:00Z',
    expires_at: '2026-12-01T00:00:00Z',
    read: false,
    dismissed: false,
    dismissible: true,
    ...over,
  }
}

export function setting(
  key: string,
  value: string | boolean,
  source = 'default',
) {
  return { key, value, source }
}

/** Routes fetch by "METHOD url"; every call is recorded so tests can assert on writes. */
export function mockApi(routes: Record<string, unknown>) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({ url, method, body: init?.body as string | undefined })
      return Promise.resolve(
        new Response(JSON.stringify(routes[`${method} ${url}`] ?? {}), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  return { calls, routes }
}
