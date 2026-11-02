import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { AutoHostList } from './AutoHostList'

function stub(names: string[], putStatus: number, calls: string[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push(`${init?.method ?? 'GET'} ${url}`)
      if (init?.method === 'PUT') {
        return Promise.resolve(
          new Response(
            putStatus === 200 ? '{"enabled":false}' : '{"error":"no"}',
            {
              status: putStatus,
            },
          ),
        )
      }
      if (url.endsWith('/auto-host'))
        return Promise.resolve(
          new Response('{"enabled":true,"hostname":"x.example.test"}', {
            status: 200,
          }),
        )
      return Promise.resolve(
        new Response(JSON.stringify(names.map((name) => ({ name }))), {
          status: 200,
        }),
      )
    }),
  )
}

function renderList() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <AllI18n>
        <AutoHostList />
      </AllI18n>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AutoHostList', () => {
  it('toggle calls PUT with enabled false', async () => {
    const calls: string[] = []
    stub(['web'], 200, calls)
    renderList()
    const sw = await screen.findByRole('switch', {
      name: 'Automatic address for web',
    })
    await waitFor(() => expect(sw.getAttribute('aria-checked')).toBe('true'))
    fireEvent.click(sw)
    await waitFor(() =>
      expect(calls).toContain('PUT /api/v1/apps/web/auto-host'),
    )
  })

  it('rolls back when the PUT fails', async () => {
    const calls: string[] = []
    stub(['web'], 500, calls)
    renderList()
    const sw = await screen.findByRole('switch', {
      name: 'Automatic address for web',
    })
    await waitFor(() => expect(sw.getAttribute('aria-checked')).toBe('true'))
    fireEvent.click(sw)
    await waitFor(() =>
      expect(calls).toContain('PUT /api/v1/apps/web/auto-host'),
    )
    await waitFor(() => expect(sw.getAttribute('aria-checked')).toBe('true'))
  })

  it('shows search past ten apps and filters', async () => {
    const names = Array.from({ length: 12 }, (_, i) => `app${i}`)
    stub(names, 200, [])
    renderList()
    const box = await screen.findByRole('searchbox', { name: 'Search apps' })
    fireEvent.change(box, { target: { value: 'app11' } })
    expect(await screen.findAllByRole('switch')).toHaveLength(1)
  })
})
