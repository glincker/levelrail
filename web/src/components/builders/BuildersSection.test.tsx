import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import deploysEn from '../../locales/en/deploys.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { BuildersSection } from './BuildersSection'

vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children?: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['nodes', 'deploys'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn, deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

const BUILDER = {
  id: 'n1',
  name: 'mac-1',
  status: 'busy',
  paused: false,
  tier: 'personal',
  os: 'darwin',
  arch: 'arm64',
  agent_version: '1.5.7',
  cpu_cores: 8,
  pressure: 'warn',
  slots_used: 1,
  slots_total: 2,
  active_builds: 1,
  waiting: 0,
  accepts_app_workloads: false,
  labels: null,
  recent_builds: 3,
  recent_failed: 0,
  avg_build_ms: 65000,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BuildersSection', () => {
  it('lists builders with tier, load and platform', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ builders: [BUILDER] }), {
            status: 200,
          }),
        ),
      ),
    )
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={i18n}>
          <BuildersSection />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    expect(await screen.findByText('mac-1')).toBeInTheDocument()
    expect(screen.getByLabelText('Tier of mac-1')).toHaveValue('personal')
    expect(screen.getByText('darwin / arm64')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Add builder' }),
    ).toBeInTheDocument()
  })

  it('shows the empty state with one add button', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ builders: [] }), { status: 200 }),
        ),
      ),
    )
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={i18n}>
          <BuildersSection />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    expect(await screen.findByText(/No builders yet/)).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Add builder' })).toHaveLength(
      1,
    )
  })
})
