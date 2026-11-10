import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import { NodeResourcesTab } from './NodeResourcesTab'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

function mockFetch(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      } as unknown as Response),
    ),
  )
}

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <NodeResourcesTab nodeId="nd_1" />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('NodeResourcesTab', () => {
  it('lists an app and a database with kind and size, and previews the drain', async () => {
    mockFetch({
      node_id: 'nd_1',
      resources: [
        {
          kind: 'app',
          name: 'web',
          detail: 'img:1',
          status: 'active',
          memory_bytes: 536870912,
        },
        {
          kind: 'database',
          name: 'main-db',
          detail: 'postgres 16',
          status: 'active',
        },
      ],
    })
    renderTab()
    expect(await screen.findByText('web')).toBeTruthy()
    expect(screen.getByText('main-db')).toBeTruthy()
    expect(screen.getByText('App')).toBeTruthy()
    expect(screen.getByText('Database')).toBeTruthy()
    expect(screen.getByText(/512(\.0)? MiB memory/)).toBeTruthy()
    expect(screen.getAllByRole('tab')).toHaveLength(2)
    fireEvent.click(screen.getByRole('tab', { name: 'Drain preview' }))
    expect(
      await screen.findByText(
        /would move the 1 app\(s\) and 1 database\(s\) you can see/,
      ),
    ).toBeTruthy()
  })

  it('shows an empty state for a node with nothing placed', async () => {
    mockFetch({ node_id: 'nd_1', resources: [] })
    renderTab()
    expect(
      await screen.findByText('Nothing you can see is placed on this node.'),
    ).toBeTruthy()
  })
})
