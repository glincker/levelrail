import { QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NodesI18n } from '../test/nodesI18n'
import type { NodeResource } from '../types/nodeDetail'
import type { NodeSecurityEvent, NodeTrustResource } from '../types/nodeTrust'
import { NodeAgentCard } from './NodeAgentCard'
import { createTestQueryClient } from '@/test/testQueryClient'

const rootRef = vi.hoisted(() => ({ value: true }))
vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => rootRef.value }))

const baseNode: NodeResource = {
  id: 'n1',
  name: 'worker-1',
  status: 'online',
  schedulable: true,
  accepts_app_workloads: true,
  accepts_build_workloads: false,
  created_at: '2026-10-01T00:00:00Z',
  is_local: false,
  cert_fingerprint: 'ab12cd',
  cert: {
    state: 'ok',
    generation: 2,
    key_origin: 'agent',
    warning_days: 21,
    critical_days: 7,
    days_remaining: 60,
  },
  agent: {
    version: 'v1.0.0',
    os: 'linux',
    arch: 'amd64',
    outdated: false,
    control_plane_version: 'v1.0.0',
  },
}

const trust = (over: Partial<NodeTrustResource>): NodeTrustResource => ({
  node_id: 'n1',
  is_local: false,
  status: { state: 'verified' },
  ca: { fingerprint: 'aabbccdd', pin_state: 'pinned' },
  certificate: {
    fingerprint: '0102030405',
    serial: '2a',
    not_after: '2026-12-01T00:00:00Z',
    days_remaining: 60,
    lifetime_used: 0.25,
    key_origin: 'agent',
  },
  renewal: {
    renewal_count: 2,
    consecutive_failures: 0,
    next_renew_estimated: true,
    next_renew_at: '2026-11-15T00:00:00Z',
  },
  validation: { window_hours: 24, accepted: 7, rejected: 2 },
  ...over,
})

const events: NodeSecurityEvent[] = [
  {
    id: 2,
    kind: 'connect',
    outcome: 'rejected',
    reason: 'expired',
    source_ip: '203.0.113.5',
    count: 4,
    first_at: '2026-10-16T10:00:00Z',
    last_at: '2026-10-16T11:00:00Z',
  },
  {
    id: 1,
    kind: 'connect',
    outcome: 'accepted',
    reason: 'mutual_tls_verified',
    source_ip: '10.0.0.2',
    count: 1,
    first_at: '2026-10-16T09:00:00Z',
    last_at: '2026-10-16T09:00:00Z',
  },
]

function stubApi(t: NodeTrustResource): string[] {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      urls.push(url)
      const body = url.includes('/security/events')
        ? url.includes('outcome=rejected')
          ? events.filter((e) => e.outcome === 'rejected')
          : events
        : t
      return Promise.resolve(
        new Response(JSON.stringify(body), { status: 200 }),
      )
    }),
  )
  return urls
}

function renderCard(node: NodeResource) {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <NodesI18n>
        <NodeAgentCard node={node} />
      </NodesI18n>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  rootRef.value = true
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('NodeAgentCard for the control plane node', () => {
  it('explains there is no agent certificate and shows no certificate rows', () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    renderCard({
      ...baseNode,
      is_local: true,
      cert: undefined,
      cert_fingerprint: undefined,
      agent: {
        version: 'v1.2.3',
        os: 'linux',
        arch: 'arm64',
        outdated: false,
        control_plane_version: 'v1.2.3',
      },
    })
    expect(screen.getByText(/no agent certificate is used/i)).toBeTruthy()
    expect(screen.getByText('v1.2.3')).toBeTruthy()
    expect(screen.getByText('linux/arm64')).toBeTruthy()
    for (const gone of [
      /never/i,
      /unknown/i,
      /not reported/i,
      /fingerprint/i,
      /last renewed/i,
      /private key/i,
      /re-enroll/i,
      /revoke/i,
    ]) {
      expect(screen.queryByText(gone)).toBeNull()
    }
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})

describe('NodeAgentCard for a remote node', () => {
  it('words the status line from the API state and groups fingerprints', async () => {
    stubApi(trust({}))
    renderCard(baseNode)
    expect(
      await screen.findByText(
        'Connection verified: mutual TLS, certificate matches this node',
      ),
    ).toBeTruthy()
    expect(screen.getByText('AA:BB:CC:DD')).toBeTruthy()
    expect(screen.getByText('01:02:03:04:05')).toBeTruthy()
    expect(screen.getByText(/Generated on the node/)).toBeTruthy()
    expect(screen.getByText('7 accepted, 2 rejected')).toBeTruthy()
    expect(screen.getByRole('button', { name: /renew now/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /re-enroll node/i })).toBeTruthy()
    expect(
      screen.getByRole('button', { name: /revoke certificate/i }),
    ).toBeTruthy()
  })

  it('never claims verified when the API reports rejections', async () => {
    stubApi(trust({ status: { state: 'rejected', reason: 'expired' } }))
    renderCard(baseNode)
    expect(
      await screen.findByText(
        /Recent connections were rejected \(certificate expired\)/,
      ),
    ).toBeTruthy()
    expect(screen.queryByText(/Connection verified/)).toBeNull()
  })

  it('says plainly that a control plane generated key needs a re-enroll', async () => {
    stubApi(
      trust({
        certificate: {
          ...trust({}).certificate!,
          key_origin: 'reenroll_required',
        },
      }),
    )
    renderCard(baseNode)
    expect(
      await screen.findByText(/Re-enroll required: this node will not renew/),
    ).toBeTruthy()
  })

  it('filters the timeline to rejected events', async () => {
    const urls = stubApi(trust({}))
    renderCard(baseNode)
    await screen.findByText(/Connection check: mutual TLS verified/)
    await userEvent.click(screen.getByRole('button', { name: 'Rejected only' }))
    await waitFor(() => {
      expect(urls.some((u) => u.includes('outcome=rejected'))).toBe(true)
    })
    await waitFor(() => {
      expect(screen.queryByText(/mutual TLS verified/)).toBeNull()
    })
    expect(screen.getByText(/certificate expired/)).toBeTruthy()
  })

  it('hides trust details and controls from non-root accounts', () => {
    rootRef.value = false
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    renderCard(baseNode)
    expect(screen.getByText(/visible to root accounts/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /revoke/i })).toBeNull()
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})
