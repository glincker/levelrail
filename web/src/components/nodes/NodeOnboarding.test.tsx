import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import type { NodeDomains, NodeOnboarding } from '../../types/nodeOnboarding'
import NodeDomainsCard from './NodeDomainsCard'
import { NodeOnboardingPanel } from './NodeOnboardingPanel'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

type Routes = Record<string, unknown>

function stubFetch(routes: Routes) {
  const fetchMock = vi.fn<typeof fetch>((input) => {
    const url = typeof input === 'string' ? input : (input as Request).url
    const hit = Object.entries(routes).find(([prefix]) =>
      url.startsWith(prefix),
    )
    return Promise.resolve({
      ok: Boolean(hit),
      status: hit ? 200 : 404,
      json: () => Promise.resolve(hit?.[1]),
      text: () => Promise.resolve(''),
    } as unknown as Response)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderWithQuery(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

const baseOnboarding: NodeOnboarding = {
  node_id: 'nd_1',
  stage: 'ready',
  reported: true,
  stale: false,
  detected: {
    provider: 'hetzner',
    region: 'eu-central',
    zone: 'fsn1-dc14',
    source: 'metadata',
  },
  domain: { provider: 'hetzner', region: 'eu-central', zone: 'fsn1-dc14' },
  checks: [
    { id: 'docker', status: 'pass', detail: 'Docker 27 answered' },
    {
      id: 'firewall',
      status: 'unknown',
      detail: 'cannot be proven from the node',
      fix: 'Allow inbound UDP 51820 from the other nodes',
    },
  ],
  summary: { pass: 1, warn: 0, fail: 0, unknown: 1, ready: true },
}

describe('NodeOnboardingPanel', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the waiting state before the agent contacts the control plane', async () => {
    stubFetch({ '/api/v1/nodes?q=': [] })
    renderWithQuery(<NodeOnboardingPanel nodeName="edge-1" />)
    expect(await screen.findByText(/Waiting for the agent/)).toBeVisible()
    expect(screen.getByText('Join token created')).toBeVisible()
    expect(screen.queryByText('Docker')).not.toBeInTheDocument()
  })

  it('shows detected labels, every check and the unverified firewall fix', async () => {
    stubFetch({
      '/api/v1/nodes?q=': [{ id: 'nd_1', name: 'edge-1' }],
      '/api/v1/nodes/nd_1/onboarding': baseOnboarding,
    })
    renderWithQuery(<NodeOnboardingPanel nodeName="edge-1" />)
    expect(
      await screen.findByText(/Detected from cloud metadata/),
    ).toBeVisible()
    expect(screen.getByText('Docker')).toBeVisible()
    expect(screen.getByText('Unverified', { selector: 'span' })).toBeVisible()
    expect(screen.getByText(/Allow inbound UDP 51820/)).toBeVisible()
    expect(screen.getByRole('button', { name: 'Re-check' })).toBeEnabled()
  })

  it('reports a failed check with its fix and a collapsed CLI command', async () => {
    stubFetch({
      '/api/v1/nodes?q=': [{ id: 'nd_1', name: 'edge-1' }],
      '/api/v1/nodes/nd_1/onboarding': {
        ...baseOnboarding,
        stage: 'needs_attention',
        checks: [
          {
            id: 'clock',
            status: 'fail',
            detail: 'the node clock differs by 3m0s',
            fix: 'Enable time synchronization on the node',
            fix_command: 'sudo timedatectl set-ntp true',
          },
        ],
        summary: { pass: 0, warn: 0, fail: 1, unknown: 0, ready: false },
      } satisfies NodeOnboarding,
    })
    renderWithQuery(<NodeOnboardingPanel nodeName="edge-1" />)
    expect(await screen.findByText('Clock skew')).toBeVisible()
    expect(screen.getByText('Failed')).toBeVisible()
    expect(screen.getByText(/Enable time synchronization/)).toBeVisible()
    expect(screen.getByText('Fix via CLI')).toBeVisible()
  })

  it('re-checks on demand', async () => {
    const fetchMock = stubFetch({
      '/api/v1/nodes?q=': [{ id: 'nd_1', name: 'edge-1' }],
      '/api/v1/nodes/nd_1/onboarding': baseOnboarding,
    })
    renderWithQuery(<NodeOnboardingPanel nodeName="edge-1" />)
    const button = await screen.findByRole('button', { name: 'Re-check' })
    const before = fetchMock.mock.calls.length
    fireEvent.click(button)
    await vi.waitFor(() =>
      expect(fetchMock.mock.calls.length).toBeGreaterThan(before),
    )
  })

  it('warns when the agent stopped reporting', async () => {
    stubFetch({
      '/api/v1/nodes?q=': [{ id: 'nd_1', name: 'edge-1' }],
      '/api/v1/nodes/nd_1/onboarding': { ...baseOnboarding, stale: true },
    })
    renderWithQuery(<NodeOnboardingPanel nodeName="edge-1" />)
    expect(await screen.findByText(/stopped reporting/)).toBeVisible()
  })
})

const domains: NodeDomains = {
  providers: [
    {
      name: 'aws',
      node_ids: ['a', 'b'],
      nodes: 2,
      online_nodes: 2,
      schedulable_nodes: 2,
      workloads: 3,
      cpu_milli: 8000,
      memory_bytes: 16 * 1024 ** 3,
      disk_free_bytes: 100 * 1024 ** 3,
      unverified_nodes: 1,
    },
  ],
  regions: [],
  zones: [],
  unlabeled_nodes: 1,
  total_nodes: 3,
  risks: [
    {
      kind: 'replicas_one_zone',
      level: 'warn',
      name: 'web',
      node_id: 'a',
      replicas: 3,
      alternatives: ['us-east-1b'],
      message: 'all 3 replicas of web run in zone us-east-1a',
    },
  ],
}

describe('NodeDomainsCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows capacity, unverified nodes and single-zone warnings', async () => {
    stubFetch({ '/api/v1/nodes/domains': domains })
    renderWithQuery(<NodeDomainsCard />)
    expect(await screen.findByText('aws')).toBeVisible()
    expect(
      screen.getByText(/8\.0 cores, 16\.0 GiB memory, 100 GiB free disk/),
    ).toBeVisible()
    expect(
      screen.getByText(/1 node has not reported its capacity/),
    ).toBeVisible()
    expect(screen.getByText(/1 of 3 nodes has no provider/)).toBeVisible()
    expect(screen.getByText(/all 3 replicas of web run in zone/)).toBeVisible()
  })

  it('renders nothing for a single-node install', async () => {
    stubFetch({ '/api/v1/nodes/domains': { ...domains, total_nodes: 1 } })
    const { container } = renderWithQuery(<NodeDomainsCard />)
    await vi.waitFor(() => expect(container.firstChild).toBeNull())
  })

  it('offers a retry when loading fails', async () => {
    stubFetch({})
    renderWithQuery(<NodeDomainsCard />)
    expect(
      await screen.findByText(/Could not load the domain overview/),
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeVisible()
  })
})
