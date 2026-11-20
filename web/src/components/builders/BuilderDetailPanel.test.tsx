import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import deploysEn from '../../locales/en/deploys.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import BuilderDetailPanel from './BuilderDetailPanel'

vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  useNavigate: () => vi.fn(),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['nodes', 'deploys'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn, deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

const base = {
  id: 'n1',
  name: 'mac-1',
  status: 'ready',
  paused: false,
  tier: 'personal',
  os: 'darwin',
  arch: 'arm64',
  agent_version: '1.5.7',
  cpu_cores: 8,
  pressure: 'ok',
  slots_used: 0,
  slots_total: 2,
  active_builds: 0,
  waiting: 0,
  accepts_app_workloads: false,
  labels: null,
  recent_builds: 1,
  recent_failed: 0,
  avg_build_ms: 30000,
  builds: [
    {
      app: 'web',
      tag: 'abc123',
      started_at: '2026-01-01T00:00:00Z',
      duration_ms: 30000,
      result: 'success',
    },
  ],
}

afterEach(() => {
  vi.unstubAllGlobals()
})

function setup(
  testBody: unknown,
  calls: string[],
  detail: Record<string, unknown> = base,
) {
  vi.stubGlobal(
    'fetch',
    vi.fn((u: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push(`${method} ${u}`)
      let body: unknown = { builders: [base] }
      if (u.endsWith('/test')) body = testBody
      else if (u.endsWith('/pause'))
        body = { ...detail, paused: true, status: 'paused' }
      else if (u.endsWith('/n1')) body = detail
      return Promise.resolve(
        new Response(JSON.stringify(body), { status: 200 }),
      )
    }),
  )
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>
        <BuilderDetailPanel nodeId="n1" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('BuilderDetailPanel', () => {
  it('shows recent builds and a passing test inline', async () => {
    const calls: string[] = []
    setup(
      {
        id: 'n1',
        name: 'mac-1',
        ok: true,
        latency_ms: 42,
        arch: 'arm64',
        os: 'darwin',
        agent_version: '1.5.7',
        disk_free_bytes: 5_000_000_000,
      },
      calls,
    )
    expect(await screen.findByText('abc123')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Test builder' }))
    expect(await screen.findByText('Builder is working')).toBeInTheDocument()
    expect(screen.getByText('Latency 42 ms')).toBeInTheDocument()
    expect(screen.getByText(/Free disk/)).toBeInTheDocument()
  })

  it('shows a failing test with its error', async () => {
    const calls: string[] = []
    setup(
      {
        id: 'n1',
        name: 'mac-1',
        ok: false,
        latency_ms: 0,
        arch: 'arm64',
        os: 'darwin',
        agent_version: '1.5.7',
        error: 'builder is offline',
      },
      calls,
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Test builder' }),
    )
    expect(await screen.findByText('Builder test failed')).toBeInTheDocument()
    expect(screen.getByText('builder is offline')).toBeInTheDocument()
  })

  it('posts pause', async () => {
    const calls: string[] = []
    setup({}, calls)
    await userEvent.click(await screen.findByRole('button', { name: 'Pause' }))
    await waitFor(() => {
      expect(calls).toContain('POST /api/v1/builders/n1/pause')
    })
  })
})
