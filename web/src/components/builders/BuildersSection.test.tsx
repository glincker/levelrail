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

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BuildersSection', () => {
  it('lists node runners with tier and hides the control plane', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              strategy: 'first',
              runners: [
                {
                  id: 'cp',
                  name: 'control',
                  kind: 'control_plane',
                  status: 'ready',
                  health: '',
                  slots_used: 0,
                  slots_total: 2,
                  waiting: 0,
                  load: 0,
                  pressure: '',
                  labels: null,
                },
                {
                  id: 'n1',
                  name: 'mac-1',
                  kind: 'node',
                  status: 'ready',
                  health: '',
                  slots_used: 1,
                  slots_total: 2,
                  active_builds: 1,
                  waiting: 0,
                  load: 0,
                  pressure: '',
                  architecture: 'arm64',
                  labels: null,
                  trust: 'personal',
                },
              ],
            }),
            { status: 200 },
          ),
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
    expect(screen.queryByText('control')).toBeNull()
    expect(screen.getByLabelText('Tier of mac-1')).toHaveValue('personal')
    expect(screen.getByText('arm64')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Add builder' }),
    ).toBeInTheDocument()
  })
})
