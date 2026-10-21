import type { AnchorHTMLAttributes, ReactNode } from 'react'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { DashboardHome } from './DashboardHome'
import { NextStepsView } from './NextSteps'
import type { NextStep } from '../../lib/dashboardHome'
import type { AppListEntry } from '../../types/appDetail'
import commonEn from '../../locales/en/common.json'
import dashboardEn from '../../locales/en/dashboard.json'

// A dedicated instance with the real dashboard.json resources loaded
// synchronously, same reasoning as AutoRollbackCard.test.tsx's own.
const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dashboard', 'common'],
  defaultNS: 'dashboard',
  fallbackNS: 'common',
  resources: { en: { dashboard: dashboardEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

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
vi.mock('../getStarted/FirstDeploy', () => ({
  FirstDeploy: () => <button type="button">Deploy sample app</button>,
}))
vi.mock('../FleetResourceChart', () => ({ FleetResourceChart: () => null }))
vi.mock('../FleetUtilizationSummary', () => ({
  FleetUtilizationSummary: () => null,
}))
vi.mock('../TopResourceConsumers', () => ({ TopResourceConsumers: () => null }))
vi.mock('../../hooks/useBrand', () => ({ useBrand: () => ({ Name: 'Acme' }) }))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => false }))

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
        headers: new Headers(),
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
        <DashboardHome
          summary={{
            total: apps.length,
            running: apps.filter((a) => a.status.variant === 'success').length,
            failing: apps.filter((a) => a.status.variant === 'destructive')
              .length,
            deploying: 0,
            stopped: 0,
            attention: 0,
            by_environment: {},
          }}
          sample={apps}
        />
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
  it('first run shows the single first-deploy path and the adaptive quick start', async () => {
    routeFetch({})
    renderHome([])
    expect(
      await screen.findByRole('button', { name: 'Deploy sample app' }),
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
      '=/api/v1/apps?status=failing&limit=100': apps,
      '/requests': SERIES,
      deploy: [],
      nodes: [],
      gpus: [],
      certificates: [],
      'system/doctor': { ok: true, checks: [] },
      'system/status': { docker_connected: true },
    })
    renderHome(apps)
    expect(await screen.findByText('All systems normal')).toBeInTheDocument()
    expect(screen.getAllByText('1 / 1')).toHaveLength(2)
    expect(
      screen.getByRole('region', { name: 'Recommended next steps' }),
    ).toBeInTheDocument()
    expect(
      await screen.findByText(/Nothing needs you\. 1 of 1 apps/),
    ).toBeInTheDocument()
    expect(screen.getByText('Your app is healthy')).toBeInTheDocument()
  })

  it('puts setup and hardening advice on one quiet line, not in incidents', async () => {
    const apps = [makeApp('web', true)]
    routeFetch({
      '=/api/v1/apps?status=failing&limit=100': apps,
      '/attention/feed': {
        items: [
          {
            id: 'docker_guard:ready',
            severity: 'info',
            kind: 'docker_guard',
            subject: 'ready',
            title: 'Docker API guard needs a decision',
            detail: 'audit mode saw no would-be denials',
            action: 'Switch to enforce',
            link: '/settings/security',
            class: 'recommendation',
          },
          {
            id: 'docker_guard:would-deny',
            severity: 'warning',
            kind: 'docker_guard',
            subject: 'would-deny',
            title: 'Docker API guard needs a decision',
            detail: 'audit mode would deny 2 requests',
            action: 'Review',
            link: '/settings/security',
            class: 'recommendation',
          },
        ],
      },
      '/requests': SERIES,
      deploy: [],
      nodes: [],
      gpus: [],
      certificates: [
        {
          domain: 'internal.example',
          status: 'expired',
          renewal: 'stalled',
          not_before: '2026-01-01T00:00:00Z',
          not_after: '2026-01-01T12:00:00Z',
          source: 'acme',
          tls_owner: 'internal',
        },
      ],
      'system/doctor': {
        ok: true,
        checks: [
          {
            code: 'control_plane_dr',
            name: 'Control plane disaster recovery',
            status: 'warn',
            message: 'off-box encrypted backups are not enabled',
            class: 'recommendation',
          },
        ],
      },
      'system/status': { docker_connected: true },
    })
    renderHome(apps)
    expect(await screen.findByText('All systems normal')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('2 recommendations')).toBeInTheDocument()
    })
    expect(screen.queryByText(/Certificate for/)).not.toBeInTheDocument()
  })

  it('ranks apps and databases together in Resources', async () => {
    const apps = [makeApp('web', true)]
    routeFetch({
      '=/api/v1/apps?status=failing&limit=100': apps,
      '=/api/v1/apps/resource-usage': [
        { name: 'web', memory_usage_bytes: 200 * 1024 ** 2, cpu_percent: 2 },
      ],
      '=/api/v1/usage/summary': {
        local_cpu_cores: 4,
        databases: [
          {
            name: 'pg-main',
            memory_usage_bytes: 600 * 1024 ** 2,
            cpu_percent: 1,
          },
        ],
        volumes: {
          scope: 'local',
          total_bytes: 0,
          unknown_count: 0,
          items: [],
        },
        backups: { total_bytes: 0, count: 0, items: [] },
      },
      '=/api/v1/nodes/resource-usage': {
        nodes: [
          {
            node_id: 'local',
            name: 'host-1',
            is_local: true,
            memory_total_bytes: 8 * 1024 ** 3,
            memory_available_bytes: 4 * 1024 ** 3,
            disk_total_bytes: 100 * 1024 ** 3,
            disk_used_bytes: 40 * 1024 ** 3,
            cpu_percent: 3,
          },
        ],
        fleet: {
          node_count: 1,
          nodes_with_memory_capacity: 1,
          nodes_with_disk_capacity: 1,
        },
      },
      '/requests': SERIES,
      deploy: [],
      nodes: [],
      gpus: [],
      certificates: [],
      'system/doctor': { ok: true, checks: [] },
      'system/status': { docker_connected: true },
    })
    renderHome(apps)
    const region = await screen.findByRole('region', { name: 'Resources' })
    await waitFor(() => {
      expect(within(region).getByText('pg-main')).toBeInTheDocument()
    })
    const rows = within(region).getAllByRole('link')
    const names = rows.map((r) => r.textContent ?? '')
    expect(names.findIndex((n) => n.includes('pg-main'))).toBeLessThan(
      names.findIndex((n) => n.includes('web')),
    )
    expect(within(region).getByText('host-1')).toBeInTheDocument()
    expect(within(region).getByText('Memory breakdown')).toBeInTheDocument()
  })

  it('populated with a failing app surfaces it with one-click actions', async () => {
    const apps = [makeApp('web', false)]
    routeFetch({
      '=/api/v1/apps?status=failing&limit=100': apps,
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
      screen.queryByRole('button', { name: 'Open logs' }),
    ).not.toBeInTheDocument()
    expect(screen.getByText('1 thing needs attention')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('1 incident')).toBeInTheDocument()
    })
  })
})

describe('NextStepsView', () => {
  const steps: NextStep[] = [
    {
      id: 'app:web',
      source: 'attention',
      tone: 'danger',
      title: 'web is failing',
      detail: 'Attention needed',
      actionLabel: 'Restart',
      spec: { kind: 'restart', app: 'web' },
    },
    {
      id: 'setup:tls',
      source: 'setup',
      stepId: 'tls',
      inProgress: false,
      href: '/settings/certificates',
    },
  ]
  const quiet = { running: 3, total: 4, deploys: 2 }

  function renderView(props: {
    steps: NextStep[]
    loading?: boolean
    percent?: number
    onRun?: (s: NextStep) => void
  }) {
    return render(
      <I18nextProvider i18n={testI18n}>
        <NextStepsView
          steps={props.steps}
          loading={props.loading ?? false}
          percent={props.percent}
          quiet={quiet}
          onRun={props.onRun ?? vi.fn()}
        />
      </I18nextProvider>,
    )
  }

  it('gives every step one action and reports which one ran', () => {
    const onRun = vi.fn()
    renderView({ steps, onRun, percent: 40 })
    expect(screen.getAllByRole('button')).toHaveLength(2)
    expect(screen.getByText('Setup 40% ready')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Restart' }))
    expect(onRun).toHaveBeenCalledWith(steps[0])
  })

  it('celebrates quietly with real numbers when nothing is left', () => {
    renderView({ steps: [] })
    expect(
      screen.getByText(
        'Nothing needs you. 3 of 4 apps running, 2 deploys in the last 24 hours.',
      ),
    ).toBeInTheDocument()
  })
})
