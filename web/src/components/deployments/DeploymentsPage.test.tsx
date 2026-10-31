import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { makeDeployment } from '../../test/deploymentFixtures'
import { parseDeploymentsSearch } from '../../lib/deploymentFilters'
import { DeploymentsPage } from './DeploymentsPage'
import deploysEn from '../../locales/en/deploys.json'
import type { Deployment } from '../../types/deployment'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['deploys'],
  defaultNS: 'deploys',
  resources: { en: { deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children?: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))

class FakeEventSource {
  static CLOSED = 2
  readyState = 1
  onopen: (() => void) | null = null
  onmessage: (() => void) | null = null
  onerror: (() => void) | null = null
  close() {}
}

const summary = {
  window: '24h',
  counts: {},
  in_progress: 0,
  needs_attention: 0,
  failure_rate_24h: null,
  duration: { median_ms: null, p95_ms: null, samples: 0 },
  per_day: [],
}

function mockApi(items: Deployment[], listStatus = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const body = url.includes('/summary')
        ? summary
        : { items: url.includes('building') ? [] : items, next_cursor: '' }
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status:
            url.includes('/summary') || url.includes('building')
              ? 200
              : listStatus,
        }),
      )
    }),
  )
}

function renderPage(search: Record<string, unknown> = {}) {
  const qc = createTestQueryClient()
  const onSearchChange = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <DeploymentsPage
          search={parseDeploymentsSearch(search)}
          onSearchChange={onSearchChange}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return { onSearchChange }
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DeploymentsPage', () => {
  it('shows a skeleton first, then the rows', async () => {
    mockApi([makeDeployment({ id: 'a' })])
    renderPage()
    expect(
      screen.getByRole('status', { name: 'Loading deployments' }),
    ).toBeInTheDocument()
    expect(
      await screen.findByText('Fix the login redirect'),
    ).toBeInTheDocument()
  })

  it('shows the empty state when nothing has ever deployed', async () => {
    mockApi([])
    renderPage()
    expect(await screen.findByTestId('page-banner-full')).toBeInTheDocument()
  })

  it('offers to clear filters when nothing matches', async () => {
    mockApi([])
    const { onSearchChange } = renderPage({ status: 'failed' })
    await userEvent.click(
      await screen.findByRole('button', { name: 'Clear filters' }),
    )
    expect(onSearchChange).toHaveBeenCalledWith({})
  })

  it('shows an error state with retry', async () => {
    mockApi([], 500)
    renderPage()
    expect(
      await screen.findByText('Deployments could not be loaded'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('writes the open row into the URL', async () => {
    mockApi([makeDeployment({ id: 'a' })])
    const { onSearchChange } = renderPage()
    await userEvent.click(await screen.findByText('Fix the login redirect'))
    expect(onSearchChange).toHaveBeenCalledWith({ d: 'a' })
  })
})
