import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import clustersEn from '../../locales/en/clusters.json'
import commonEn from '../../locales/en/common.json'
import { DeploymentRow } from './DeploymentRow'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('@/components/ui/RoleGate', () => ({
  RoleGate: ({ children }: { children: ReactNode }) => <>{children}</>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'clusters'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, clusters: clustersEn } },
  interpolation: { escapeValue: false },
})

const dep = {
  app: 'web',
  namespace: 'levelrail-managed',
  image: 'nginx',
  port: 80,
  replicas: 1,
  allow_root: false,
  objects: [],
}

const crashing = {
  app: 'web',
  namespace: 'levelrail-managed',
  found: true,
  ready: false,
  replicas: 1,
  ready_replicas: 0,
  updated_replicas: 1,
  conditions: [],
  pods: [
    {
      name: 'web-1',
      phase: 'Running',
      reason: 'CrashLoopBackOff',
      message: 'back-off restarting failed container',
    },
  ],
}

function wrap(node: ReactNode) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DeploymentRow', () => {
  it('shows the crash reason and message with ready n/m', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify(crashing), { status: 200 }),
        ),
      ),
    )
    wrap(<DeploymentRow cluster="c1" deployment={dep} />)
    expect(await screen.findByText('CrashLoopBackOff')).toBeInTheDocument()
    expect(screen.getByText('Ready 0/1')).toBeInTheDocument()
    expect(
      screen.getByText('back-off restarting failed container'),
    ).toBeInTheDocument()
  })

  it('requires the typed app name before removing', async () => {
    const fetchMock = vi.fn((_u: string, init?: RequestInit) =>
      Promise.resolve(
        new Response(
          JSON.stringify(init?.method === 'DELETE' ? { app: 'web' } : crashing),
          { status: 200 },
        ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    wrap(<DeploymentRow cluster="c1" deployment={dep} />)

    await user.click(screen.getByRole('button', { name: 'Remove web' }))
    const confirm = await screen.findByRole('button', { name: 'Remove app' })
    expect(confirm).toBeDisabled()
    await user.type(screen.getByRole('textbox'), 'web')
    expect(confirm).toBeEnabled()
    await user.click(confirm)
    await waitFor(() => {
      expect(fetchMock.mock.calls.some((c) => c[1]?.method === 'DELETE')).toBe(
        true,
      )
    })
  })
})
