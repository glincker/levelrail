import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { makeDeployment } from '../../test/deploymentFixtures'
import { parseDeploymentsSearch } from '../../lib/deploymentFilters'
import { DeploymentsPage } from './DeploymentsPage'
import deploysEn from '../../locales/en/deploys.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const stream = vi.hoisted(() => ({ state: 'live' }))
vi.mock('../../hooks/useDeploymentsStream', () => ({
  useDeploymentsStream: () => ({
    state: stream.state,
    pending: [],
    flushPending: () => undefined,
  }),
}))
vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children?: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['deploys'],
  defaultNS: 'deploys',
  resources: { en: { deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

const summary = {
  window: '24h',
  counts: {},
  in_progress: 0,
  needs_attention: 0,
  failure_rate_24h: null,
  duration: { median_ms: null, p95_ms: null, samples: 0 },
  per_day: [],
}
const runner = {
  id: 'local',
  name: 'Control plane',
  kind: 'control_plane',
  status: 'ready',
  health: 'ok',
  slots_used: 0,
  slots_total: 4,
  waiting: 0,
  load: 0.1,
  pressure: 'none',
  labels: null,
}

function mockApi(opts: { items: unknown[]; running?: boolean }) {
  const queue = {
    state: 'running',
    drained: false,
    slots: { used: opts.running ? 1 : 0, total: 4 },
    running: opts.running
      ? [{ app: 'web', phase: 'building', elapsed_seconds: 5 }]
      : [],
    queued: [],
    recent: [],
  }
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      let body: unknown = {
        items: url.includes('building') ? [] : opts.items,
        next_cursor: '',
      }
      if (url.includes('/summary')) body = summary
      else if (url.includes('/builds/queue')) body = queue
      else if (url.includes('/builds/runners'))
        body = { strategy: 'first', runners: [runner] }
      return Promise.resolve(
        new Response(JSON.stringify(body), { status: 200 }),
      )
    }),
  )
}

function renderPage() {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <DeploymentsPage
          search={parseDeploymentsSearch({})}
          onSearchChange={vi.fn()}
          tabs={<nav aria-label="Sections">tabs</nav>}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

beforeEach(() => {
  stream.state = 'live'
  vi.stubGlobal(
    'EventSource',
    class {
      close() {}
    },
  )
})
afterEach(() => {
  vi.unstubAllGlobals()
  window.localStorage.clear()
})

describe('Deployments page states', () => {
  it('empty: one primary button, no tabs, no runners, no pill', async () => {
    mockApi({ items: [] })
    renderPage()
    await screen.findByText('No deployments yet')
    expect(screen.getAllByText('Deploy an app')).toHaveLength(1)
    expect(screen.queryByRole('navigation', { name: 'Sections' })).toBeNull()
    expect(screen.queryByText('Build runners')).toBeNull()
    expect(screen.queryByText('Live')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Queue actions' })).toBeNull()
    expect(
      screen.getByRole('link', { name: 'Add a build node to move them off.' }),
    ).toBeInTheDocument()
  })

  it('idle: tabs and the compact runner strip', async () => {
    mockApi({ items: [makeDeployment({ id: 'a' })] })
    renderPage()
    expect(await screen.findAllByTestId('runner-strip-item')).toHaveLength(1)
    expect(screen.getByRole('navigation', { name: 'Sections' })).toBeVisible()
    expect(screen.queryByTestId('runner-card')).toBeNull()
    expect(screen.queryByText('Live')).toBeNull()
  })

  it('active: full runner cards while builds run', async () => {
    mockApi({ items: [makeDeployment({ id: 'a' })], running: true })
    renderPage()
    expect(await screen.findAllByTestId('runner-card')).toHaveLength(1)
    expect(
      await screen.findByRole('button', { name: /Kill all builds/ }),
    ).toBeInTheDocument()
  })

  it('error: a dropped stream shows the warning pill', async () => {
    stream.state = 'reconnecting'
    mockApi({ items: [makeDeployment({ id: 'a' })] })
    renderPage()
    expect(await screen.findByText('Reconnecting')).toBeInTheDocument()
  })
})
