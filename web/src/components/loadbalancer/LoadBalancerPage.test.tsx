import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Brand } from '../../types/brand'
import type { LiveUpstream } from '../../queries/loadBalancerLive'
import { formFromConfig } from '../../lib/loadBalancer'
import { ChangeSummaryBar } from './ChangeSummaryBar'
import { summarizeChanges, predictEffect } from './changes'
import { DistributionBar } from './WeightSliders'
import { LbTopology } from './LbTopology'
import { LoadBalancerPage } from './LoadBalancerPage'
import { presetById } from './presets'
import { tokenize } from './highlight'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: 'https://test.example/docs',
    DiscussionsURL: '',
  }),
}))

afterEach(() => vi.unstubAllGlobals())

function up(id: number, patch: Partial<LiveUpstream> = {}): LiveUpstream {
  return {
    id: `demo#${id}`,
    dial: `127.0.0.1:3000${id}`,
    replica: id,
    weight: 0,
    state: 'healthy',
    healthy: true,
    active_connections: 2,
    fails: 0,
    latency_ms: 40,
    ...patch,
  }
}
const down = (id: number) =>
  up(id, { state: 'unhealthy', healthy: false, reason: 'status 500, want 200' })

function renderTopology(upstreams: LiveUpstream[], onSelect = vi.fn()) {
  render(
    <LbTopology
      upstreams={upstreams}
      algorithm="least_conn"
      paused
      onPausedChange={() => {}}
      onSelect={onSelect}
    />,
  )
  return onSelect
}

describe('LbTopology states', () => {
  it('healthy: every node is a labelled button and the table twin lists them', () => {
    renderTopology([up(0), up(1)])
    expect(
      screen.getAllByRole('button', { name: /Open details/ }),
    ).toHaveLength(2)
    const table = screen.getByRole('table', { hidden: true })
    expect(within(table).getAllByRole('row', { hidden: true })).toHaveLength(3)
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('degraded: shows the reason and a non-color state word', () => {
    renderTopology([up(0), down(1)])
    const node = screen.getByRole('button', { name: /127.0.0.1:30001/ })
    expect(node).toHaveAccessibleName(/Down, status 500, want 200/)
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('all unhealthy: says requests may fail', () => {
    renderTopology([down(0), down(1)])
    expect(screen.getByRole('status')).toHaveTextContent(/may fail/)
  })

  it('no upstreams: teaches what happens next', () => {
    renderTopology([])
    expect(screen.getByText(/No upstreams yet/)).toBeInTheDocument()
  })

  it('nodes open with Enter and Space', async () => {
    const onSelect = renderTopology([up(0), up(1)])
    const nodes = screen.getAllByRole('button', { name: /Open details/ })
    nodes[0]?.focus()
    await userEvent.keyboard('{Enter}')
    await userEvent.keyboard(' ')
    expect(onSelect).toHaveBeenCalledWith('demo#0')
    expect(onSelect).toHaveBeenCalledTimes(2)
  })
})

describe('DistributionBar', () => {
  it('describes percentages from weights', () => {
    render(<DistributionBar weights={[9, 1]} />)
    expect(screen.getByRole('img')).toHaveAccessibleName(
      'Traffic split: replica 0 90%, replica 1 10%',
    )
  })
})

describe('ChangeSummaryBar', () => {
  const before = formFromConfig(
    { algorithm: 'round_robin', active_health: { path: '/', interval: '10s' } },
    2,
  )

  it('shows before and after chips, the effect, and saves', async () => {
    const after = { ...before, healthInterval: '5s' }
    const chips = summarizeChanges(before, after)
    const onSave = vi.fn()
    render(
      <ChangeSummaryBar
        chips={chips}
        effect={predictEffect(after, chips)}
        saving={false}
        creating={false}
        onSave={onSave}
        onRevert={() => {}}
      />,
    )
    const chip = screen.getByText('Check interval').closest('li')
    expect(chip).toHaveTextContent('10s')
    expect(chip).toHaveTextContent('5s')
    await userEvent.click(screen.getByRole('button', { name: /Save/ }))
    expect(onSave).toHaveBeenCalledOnce()
  })

  it('asks before saving a risky change', async () => {
    const after = { ...before, healthEnabled: false }
    const chips = summarizeChanges(before, after)
    const onSave = vi.fn()
    render(
      <ChangeSummaryBar
        chips={chips}
        effect=""
        saving={false}
        creating={false}
        onSave={onSave}
        onRevert={() => {}}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: /Save/ }))
    expect(onSave).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Save anyway' }))
    expect(onSave).toHaveBeenCalledOnce()
  })

  it('S saves outside inputs', async () => {
    const chips = summarizeChanges(before, { ...before, healthInterval: '5s' })
    const onSave = vi.fn()
    render(
      <ChangeSummaryBar
        chips={chips}
        effect=""
        saving={false}
        creating={false}
        onSave={onSave}
        onRevert={() => {}}
      />,
    )
    await userEvent.keyboard('s')
    expect(onSave).toHaveBeenCalledOnce()
  })

  it('a blocker disables save', () => {
    const chips = summarizeChanges(before, { ...before, healthInterval: '5s' })
    render(
      <ChangeSummaryBar
        chips={chips}
        effect=""
        blocker="Path must start with /"
        saving={false}
        creating={false}
        onSave={() => {}}
        onRevert={() => {}}
      />,
    )
    expect(screen.getByRole('button', { name: /Save/ })).toBeDisabled()
    expect(screen.getByText('Path must start with /')).toBeInTheDocument()
  })
})

describe('highlight', () => {
  it('classifies comments, strings, keys and numbers', () => {
    const kinds = tokenize('# note\nname = "a"\nport = 80').map((t) => t.kind)
    expect(kinds).toEqual(
      expect.arrayContaining(['comment', 'key', 'string', 'number']),
    )
  })
})

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

interface Stub {
  configured: boolean
  historyStatus?: number
  puts: unknown[]
}

function stubFetch(stub: Stub) {
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      (input, init) => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.toString()
              : input.url
        if (init?.method === 'PUT') {
          const config = JSON.parse(init.body as string) as unknown
          stub.puts.push(config)
          stub.configured = true
          return Promise.resolve(
            jsonResponse({
              app_name: 'demo',
              configured: true,
              config,
              algorithms: [],
              export_formats: [],
            }),
          )
        }
        if (url.includes('/loadbalancer/history')) {
          if (stub.historyStatus) {
            return Promise.resolve(
              jsonResponse({ error: 'nope' }, stub.historyStatus),
            )
          }
          return Promise.resolve(jsonResponse({ upstreams: [] }))
        }
        if (url.endsWith('/loadbalancer/status')) {
          return Promise.resolve(
            jsonResponse({
              service: 'demo',
              algorithm: 'least_conn',
              ready: true,
              reason: 'Ready',
              message: '',
              observed_at: '2026-01-01T00:00:00Z',
              upstreams: [up(0), down(1)],
            }),
          )
        }
        if (url.endsWith('/loadbalancer')) {
          return Promise.resolve(
            jsonResponse({
              app_name: 'demo',
              configured: stub.configured,
              config: stub.configured
                ? {
                    algorithm: 'least_conn',
                    active_health: { path: '/healthz' },
                    passive_health: { max_fails: 3 },
                    drain_timeout: '15s',
                  }
                : undefined,
              algorithms: [],
              export_formats: [],
            }),
          )
        }
        throw new Error(`unexpected fetch: ${url}`)
      },
    ),
  )
}

function renderPage(replicas = 2) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <LoadBalancerPage appName="demo" replicas={replicas} />
    </QueryClientProvider>,
  )
}

describe('LoadBalancerPage', () => {
  it('empty state creates the recommended preset in one click', async () => {
    const stub: Stub = { configured: false, puts: [] }
    stubFetch(stub)
    renderPage()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Create recommended setup' }),
    )
    await waitFor(() => expect(stub.puts).toHaveLength(1))
    expect(stub.puts[0]).toEqual(presetById('balanced')?.config)
  })

  it('picking a preset from the empty state fills a draft without saving', async () => {
    const stub: Stub = { configured: false, puts: [] }
    stubFetch(stub)
    renderPage()
    await userEvent.click(
      await screen.findByRole('button', { name: /Canary 90\/10/ }),
    )
    expect(
      await screen.findByRole('region', { name: 'Unsaved changes' }),
    ).toBeInTheDocument()
    expect(stub.puts).toHaveLength(0)
    expect(screen.getByRole('button', { name: /Create/ })).toBeInTheDocument()
  })

  it('degrades when the history endpoint is missing', async () => {
    const stub: Stub = { configured: true, historyStatus: 404, puts: [] }
    stubFetch(stub)
    renderPage()
    const check = await screen.findByRole('button', { name: /Check now/ })
    await waitFor(() => expect(check).toBeDisabled())
    expect(check).toHaveAttribute('title', 'Needs a newer control plane')
    expect(screen.queryByRole('img', { name: /checks/ })).toBeNull()
    expect(screen.getByText(/1 of 2|of/)).toBeInTheDocument()
  })

  it('keyboard opens the drawer with actions', async () => {
    const stub: Stub = { configured: true, puts: [] }
    stubFetch(stub)
    renderPage()
    const node = await screen.findByRole('button', {
      name: /127.0.0.1:30000: Healthy/,
    })
    node.focus()
    await userEvent.keyboard('{Enter}')
    const drawer = await screen.findByRole('dialog')
    expect(
      within(drawer).getByRole('button', { name: 'Drain' }),
    ).toBeInTheDocument()
    expect(
      within(drawer).getByRole('button', { name: 'Disable' }),
    ).toBeInTheDocument()
  })

  it('shows one status pill and the healthy count', async () => {
    const stub: Stub = { configured: true, puts: [] }
    stubFetch(stub)
    renderPage()
    expect(await screen.findByText('Degraded')).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByText(/healthy$/, { selector: 'p' })).toHaveTextContent(
        '1 of 2 healthy',
      ),
    )
  })
})
