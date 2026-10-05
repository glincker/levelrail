import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { DashboardHome } from './DashboardHome'
import { NeedsAttentionView } from './NeedsAttention'
import { platformPill } from '../../lib/fleetStatus'
import type { AppListEntry } from '../../types/appDetail'
import type { AttentionItem } from '../../lib/attention'
import {
  attentionToSuggestions,
  type ActionSpec,
} from '../../lib/fleetSuggestions'
import dashboardEn from '../../locales/en/dashboard.json'

// A dedicated instance with the real dashboard.json resources loaded
// synchronously, same reasoning as AutoRollbackCard.test.tsx's own.
const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dashboard'],
  defaultNS: 'dashboard',
  resources: { en: { dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})
const dashboardT = testI18n.getFixedT('en', 'dashboard')

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})
vi.mock('../CreateResourceWizard', () => ({
  CreateResourceWizard: ({ trigger }: { trigger: ReactNode }) => <>{trigger}</>,
}))
vi.mock('../setup/SetupWizard', () => ({
  SetupWizard: () => <div>wizard</div>,
}))
vi.mock('../FleetResourceChart', () => ({ FleetResourceChart: () => null }))
vi.mock('../FleetUtilizationSummary', () => ({
  FleetUtilizationSummary: () => null,
}))
vi.mock('../TopResourceConsumers', () => ({ TopResourceConsumers: () => null }))
vi.mock('../../hooks/useBrand', () => ({ useBrand: () => ({ Name: 'Acme' }) }))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => false }))

const ONBOARDING = { completed: true, current_step: '' } as never

function makeApp(name: string, healthy: boolean): AppListEntry {
  return {
    name,
    image: 'nginx',
    port: 80,
    status: healthy
      ? { label: 'Healthy', variant: 'success' }
      : { label: 'Attention needed', variant: 'destructive' },
  } as AppListEntry
}

function routeFetch(handlers: Record<string, unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const key =
        `=${String(url)}` in handlers
          ? `=${String(url)}`
          : Object.keys(handlers).find(
              (k) => !k.startsWith('=') && String(url).includes(k),
            )
      const ok = key !== undefined
      return Promise.resolve({
        ok,
        status: ok ? 200 : 404,
        json: () => Promise.resolve(ok ? handlers[key] : { error: 'nf' }),
      } as unknown as Response)
    }),
  )
}

function renderHome(apps: AppListEntry[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <DashboardHome apps={apps} onboarding={ONBOARDING} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

const SERIES = {
  app: 'web',
  summary: {
    has_traffic: true,
    rate_per_sec: 3,
    error_rate_5xx: 0,
    p95_ms: 50,
  },
  points: [],
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DashboardHome', () => {
  it('first run shows a single strong CTA and the adaptive quick start', () => {
    routeFetch({})
    renderHome([])
    expect(
      screen.getByRole('button', { name: 'Deploy your first app' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Import anything')).toBeInTheDocument()
    expect(screen.getByText('Deploy a template')).toBeInTheDocument()
    expect(screen.getByText('Connect Git')).toBeInTheDocument()
    expect(screen.queryByText('Add a node')).not.toBeInTheDocument()
    expect(screen.getByText('What Acme does')).toBeInTheDocument()
    expect(
      screen.getByText(/Rolling, recreate, or blue-green strategy/),
    ).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('tab', { name: /Eight managed databases/ }),
    )
    expect(screen.getByText(/ClickHouse/)).toBeInTheDocument()
  })

  it('populated with no attention shows a healthy pill, tiles and app quick start', async () => {
    const apps = [makeApp('web', true)]
    routeFetch({
      '=/api/v1/apps': apps,
      '/requests': SERIES,
      deploy: [],
      nodes: [],
      gpus: [],
      certificates: [],
      'system/doctor': { ok: true, checks: [] },
      'system/status': { docker_connected: true },
    })
    renderHome(apps)
    expect(await screen.findByText('All systems healthy')).toBeInTheDocument()
    expect(screen.getByText('1 / 1')).toBeInTheDocument()
    expect(screen.getByText('Add a node')).toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Needs attention' }),
    ).not.toBeInTheDocument()
  })

  it('populated with a failing app surfaces it with one-click actions', async () => {
    const apps = [makeApp('web', false)]
    routeFetch({
      '=/api/v1/apps': apps,
      '/requests': SERIES,
      deploy: [],
      nodes: [],
      gpus: [],
      certificates: [],
      'system/doctor': { ok: true, checks: [] },
      'system/status': { docker_connected: true },
    })
    renderHome(apps)
    expect(await screen.findByText('web is failing')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Open logs' }),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('1 needs attention')).toBeInTheDocument()
    })
  })
})

describe('NeedsAttentionView', () => {
  const items: AttentionItem[] = Array.from({ length: 6 }, (_, i) => ({
    id: `app:a${i}`,
    severity: 'critical',
    title: `a${i}`,
    detail: 'Attention needed',
    target: { kind: 'app', name: `a${i}` },
  }))

  function renderView(props: {
    specs: ReturnType<typeof attentionToSuggestions>
    loading: boolean
    onRun: (spec: ActionSpec) => void
  }) {
    return render(
      <I18nextProvider i18n={testI18n}>
        <NeedsAttentionView {...props} />
      </I18nextProvider>,
    )
  }

  it('renders a skeleton while loading', () => {
    const { container } = renderView({
      specs: [],
      loading: true,
      onRun: vi.fn(),
    })
    expect(container.querySelector('[aria-hidden="true"]')).not.toBeNull()
  })

  it('renders nothing when there is nothing to fix', () => {
    const { container } = renderView({
      specs: [],
      loading: false,
      onRun: vi.fn(),
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('caps at four items and links to see all', () => {
    renderView({
      specs: attentionToSuggestions(items, dashboardT),
      loading: false,
      onRun: vi.fn(),
    })
    expect(screen.getAllByRole('button', { name: 'Restart' })).toHaveLength(4)
    expect(screen.getByRole('link', { name: 'See all 6' })).toHaveAttribute(
      'href',
      '/status',
    )
  })

  it('shows an Ask AI action alongside the canned fix for every item', () => {
    renderView({
      specs: attentionToSuggestions(items, dashboardT),
      loading: false,
      onRun: vi.fn(),
    })
    expect(screen.getAllByRole('button', { name: 'Ask AI' })).toHaveLength(4)
  })
})

describe('platformPill', () => {
  it('summarizes severity', () => {
    expect(platformPill([]).tone).toBe('success')
    const w = { severity: 'warning' } as AttentionItem
    const c = { severity: 'critical' } as AttentionItem
    expect(platformPill([w])).toEqual({ tone: 'warning', label: '1 warning' })
    expect(platformPill([w, c, c])).toEqual({
      tone: 'danger',
      label: '2 need attention',
    })
  })
})
