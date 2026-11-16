import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import deploysEn from '../../locales/en/deploys.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { RunnersPanel } from './RunnersPanel'
import { RunnerChip } from './RunnerChip'
import { RunnerPreferenceCard } from './RunnerPreferenceCard'
import type { BuildRunner } from '../../queries/buildRunners'

const canDeploy = vi.hoisted(() => ({ value: true }))
vi.mock('../../hooks/useCanDeploy', () => ({
  useCanDeploy: () => canDeploy.value,
}))
vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children?: ReactNode }) => <a href="/">{children}</a>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['deploys'],
  defaultNS: 'deploys',
  resources: { en: { deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

function wrap(ui: ReactNode) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
    </QueryClientProvider>,
  )
}

const controlPlane: BuildRunner = {
  id: 'local',
  name: 'Control plane',
  kind: 'control_plane',
  status: 'busy',
  health: 'ok',
  slots_used: 1,
  slots_total: 4,
  waiting: 2,
  load: 0.5,
  pressure: 'none',
  architecture: 'arm64',
  cpu_cores: 8,
  labels: null,
  last_build_at: '2026-11-15T10:00:00Z',
  last_build_app: 'web',
  last_build_status: 'succeeded',
  cache_bytes: 2048,
}
const node: BuildRunner = {
  ...controlPlane,
  id: 'n1',
  name: 'builder-1',
  kind: 'node',
  status: 'pressure',
  cache_bytes: undefined,
  labels: ['gpu'],
}

function mockFetch(handler: (url: string, init?: RequestInit) => Response) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve(handler(url, init)),
    ),
  )
}
const ok = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200 })

afterEach(() => {
  vi.unstubAllGlobals()
  canDeploy.value = true
})

describe('RunnersPanel', () => {
  it('renders one card per runner with capacity, status and labels', async () => {
    mockFetch(() =>
      ok({ strategy: 'least_loaded', runners: [controlPlane, node] }),
    )
    wrap(<RunnersPanel />)
    expect(await screen.findAllByTestId('runner-card')).toHaveLength(2)
    expect(screen.getByText('Strategy: least loaded')).toBeInTheDocument()
    expect(screen.getAllByText('1 of 4 slots (25%)')).toHaveLength(2)
    expect(screen.getAllByText('2 waiting')).toHaveLength(2)
    expect(screen.getByText('Under pressure')).toBeInTheDocument()
    expect(screen.getByText('gpu')).toBeInTheDocument()
    expect(screen.getByText(/Build cache/)).toBeInTheDocument()
    const bars = screen.getAllByRole('progressbar')
    expect(bars[0]).toHaveAttribute('aria-valuenow', '25')
  })

  it('explains how to add a build node when only the control plane exists', async () => {
    mockFetch(() => ok({ strategy: 'first', runners: [controlPlane] }))
    wrap(<RunnersPanel />)
    expect(await screen.findByText('Add a build node')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Add build node' }),
    ).toBeInTheDocument()
  })

  it.each([[[]], [null]])('treats %j runners as empty', async (runners) => {
    mockFetch(() => ok({ strategy: 'first', runners }))
    wrap(<RunnersPanel />)
    expect(await screen.findByText('Add a build node')).toBeInTheDocument()
    expect(screen.queryByTestId('runner-card')).not.toBeInTheDocument()
  })

  it('shows a skeleton while loading and an error with retry', async () => {
    let calls = 0
    mockFetch(() => {
      calls += 1
      return calls === 1
        ? new Response('boom', { status: 500 })
        : ok({ strategy: 'first', runners: [controlPlane, node] })
    })
    wrap(<RunnersPanel />)
    expect(screen.getByRole('status')).toBeInTheDocument()
    await userEvent.click(await screen.findByRole('button', { name: 'Reload' }))
    expect(await screen.findAllByTestId('runner-card')).toHaveLength(2)
  })
})

describe('RunnerChip', () => {
  it('shows the runner and queue wait', () => {
    wrap(
      <RunnerChip
        info={{
          runner_name: 'builder-1',
          runner_reason: 'least loaded',
          queue_wait_ms: 4200,
        }}
      />,
    )
    expect(screen.getByText('Built on builder-1')).toBeInTheDocument()
    expect(screen.getByText('Queued 4s')).toBeInTheDocument()
  })

  it('renders nothing when the fields are absent', () => {
    const { container } = wrap(<RunnerChip info={{ queue_wait_ms: 0 }} />)
    expect(container).toBeEmptyDOMElement()
    const none = wrap(<RunnerChip info={null} />)
    expect(none.container).toBeEmptyDOMElement()
  })
})

describe('RunnerPreferenceCard', () => {
  function prefFetch(sent: { body?: string; method?: string }[]) {
    mockFetch((url, init) => {
      if (url.includes('/builds/runners')) {
        return ok({ strategy: 'first', runners: [controlPlane, node] })
      }
      if (init?.method === 'PUT' || init?.method === 'DELETE') {
        sent.push({
          body: init.body as string | undefined,
          method: init.method,
        })
        if (
          init.method === 'PUT' &&
          typeof init.body === 'string' &&
          init.body.includes('"pinned":true')
        ) {
          return new Response(JSON.stringify({ error: 'node is draining' }), {
            status: 400,
          })
        }
        return ok({ app: 'web', node_id: 'n1', pinned: false, labels: ['gpu'] })
      }
      return ok({ app: 'web', node_id: '', pinned: false, labels: [] })
    })
  }

  it('saves the node and labels with PUT', async () => {
    const sent: { body?: string; method?: string }[] = []
    prefFetch(sent)
    wrap(<RunnerPreferenceCard appName="web" />)
    await screen.findByRole('option', { name: 'builder-1' })
    await userEvent.selectOptions(screen.getByLabelText('Build node'), 'n1')
    await userEvent.type(screen.getByLabelText('Labels'), 'gpu')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(sent[0]?.method).toBe('PUT')
    })
    expect(JSON.parse(sent[0]?.body ?? '{}')).toEqual({
      node_id: 'n1',
      pinned: false,
      labels: ['gpu'],
    })
  })

  it('shows the server error verbatim', async () => {
    prefFetch([])
    wrap(<RunnerPreferenceCard appName="web" />)
    await screen.findByRole('option', { name: 'builder-1' })
    await userEvent.selectOptions(screen.getByLabelText('Build node'), 'n1')
    await userEvent.click(
      screen.getByRole('switch', { name: 'Pin to this node' }),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'node is draining',
    )
  })

  it('disables the controls without deploy ability', async () => {
    canDeploy.value = false
    prefFetch([])
    wrap(<RunnerPreferenceCard appName="web" />)
    expect(await screen.findByLabelText('Build node')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(
      screen.getByText('You need deploy access to change this.'),
    ).toBeInTheDocument()
  })
})
