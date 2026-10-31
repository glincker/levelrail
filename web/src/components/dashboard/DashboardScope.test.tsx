import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AppsPanel } from './AppsPanel'
import { FleetTiles } from './FleetTiles'
import { RecentActivity } from './RecentActivity'
import { ResourcesView } from './ResourcesSection'
import { InstanceWideLabel, ScopeIndicator } from './ScopeIndicator'
import { StatusStrip } from './StatusStrip'
import { RecentAlertsCard } from '../RecentAlertsCard'
import { ResourceAdvisorTile } from '../resourceAdvisor/ResourceAdvisorTile'
import { SleepFleetCard } from './SleepFleetCard'
import {
  getProjectScope,
  resetProjectScopeForTests,
  setProjectScope,
} from '../../lib/projectScope'
import { resetEnvironmentScopeForTests } from '../../lib/environmentScope'
import type { AppsSummary } from '../../queries/appPages'
import type { DashboardResources } from '../../queries/dashboardResources'
import commonEn from '../../locales/en/common.json'
import dashboardEn from '../../locales/en/dashboard.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
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
  CreateResourceWizard: () => null,
}))
vi.mock('../../hooks/useBrand', () => ({ useBrand: () => ({ Name: 'Acme' }) }))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => false }))
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => ({ data: ['resource-advisor'] }),
}))
vi.mock('../../lib/experimental', () => ({ isFeatureVisible: () => true }))

const attention = vi.hoisted(() => ({
  summary: {
    level: 'ok',
    count: 0,
    visible: [] as unknown[],
    snoozed: [] as unknown[],
    recommendations: [] as unknown[],
    snoozedRecommendations: [] as unknown[],
    isLoading: false,
  },
}))
vi.mock('../shell/useAttentionSummary', () => ({
  useAttentionSummary: () => attention.summary,
}))

const SUMMARY: AppsSummary = {
  total: 0,
  running: 0,
  failing: 0,
  deploying: 0,
  stopped: 0,
  attention: 0,
  by_environment: {},
}

let urls: string[] = []

function stubApi(routes: Record<string, unknown> = {}) {
  urls = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      urls.push(String(url))
      const key = Object.keys(routes).find((k) => String(url).includes(k))
      const body = key === undefined ? [] : routes[key]
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: new Headers(),
        json: () => Promise.resolve(body),
      } as unknown as Response)
    }),
  )
}

function renderUi(ui: ReactNode) {
  const qc = createTestQueryClient()
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
    </I18nextProvider>,
  )
}

const hit = (needle: string) => urls.some((u) => u.includes(needle))

beforeEach(() => {
  window.localStorage.clear()
  resetProjectScopeForTests()
  resetEnvironmentScopeForTests()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('scope indicator', () => {
  it('renders nothing without a scope', () => {
    stubApi()
    const { container } = renderUi(<ScopeIndicator />)
    expect(container.textContent).toBe('')
  })

  it('names the project and clears it in one click', async () => {
    stubApi({ '/api/v1/projects': [{ id: 'p1', name: 'Shop' }] })
    setProjectScope('p1')
    renderUi(<ScopeIndicator />)
    expect(await screen.findByText('Showing Shop')).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Clear scope, show all projects' }),
    )
    expect(getProjectScope()).toBe('')
    await waitFor(() => {
      expect(screen.queryByText(/Showing/)).not.toBeInTheDocument()
    })
  })
})

describe('instance-wide label', () => {
  it('shows All projects only while a scope is picked', () => {
    stubApi()
    const { rerender } = renderUi(<InstanceWideLabel />)
    expect(screen.queryByText('All projects')).not.toBeInTheDocument()
    setProjectScope('p1')
    rerender(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={new QueryClient()}>
          <InstanceWideLabel />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(screen.getByText('All projects')).toBeInTheDocument()
  })

  it('marks the host gauges but not the scoped consumers', () => {
    stubApi()
    setProjectScope('p1')
    const data: DashboardResources = {
      isPending: false,
      gauges: [{ id: 'n1', name: 'node-1', isLocal: true }],
      consumers: [],
      breakdown: undefined,
      databaseCount: 0,
    }
    renderUi(<ResourcesView data={data} />)
    expect(screen.getAllByText('All projects')).toHaveLength(1)
  })

  it('marks the nodes tile as instance-wide', async () => {
    stubApi()
    setProjectScope('p1')
    renderUi(<FleetTiles summary={SUMMARY} sample={[]} />)
    expect(await screen.findAllByText('All projects')).toHaveLength(1)
  })
})

describe('Start widgets follow the scope', () => {
  it('requests scoped traffic for the fleet tiles', async () => {
    stubApi()
    setProjectScope('p1')
    renderUi(<FleetTiles summary={SUMMARY} sample={[]} />)
    await waitFor(() => {
      expect(urls).toContain('/api/v1/apps-metrics?project=p1')
    })
  })

  it('requests unscoped traffic without a scope', async () => {
    stubApi()
    renderUi(<FleetTiles summary={SUMMARY} sample={[]} />)
    await waitFor(() => {
      expect(urls).toContain('/api/v1/apps-metrics')
    })
  })

  it('scopes the apps list', async () => {
    stubApi()
    setProjectScope('p1')
    renderUi(<AppsPanel />)
    await waitFor(() => {
      expect(urls).toContain('/api/v1/apps?project=p1')
    })
  })

  it('scopes the recent alerts panel', async () => {
    stubApi()
    setProjectScope('p1')
    renderUi(<RecentAlertsCard />)
    await waitFor(() => {
      expect(hit('/api/v1/alert-history?')).toBe(true)
    })
    expect(urls.find((u) => u.includes('alert-history'))).toContain(
      'project=p1',
    )
  })

  it('scopes the advisor summary', async () => {
    stubApi()
    setProjectScope('p1')
    renderUi(<ResourceAdvisorTile />)
    await waitFor(() => {
      expect(urls).toContain('/api/v1/resource-advisor/summary?project=p1')
    })
  })

  it('scopes the sleeping apps card', async () => {
    stubApi()
    setProjectScope('p1')
    renderUi(<SleepFleetCard />)
    await waitFor(() => {
      expect(urls).toContain('/api/v1/sleep?project=p1')
    })
  })
})

describe('empty scope', () => {
  it('explains the empty scope and offers to create an app and to clear it', async () => {
    stubApi({ '/api/v1/projects': [{ id: 'p1', name: 'Shop' }] })
    setProjectScope('p1')
    renderUi(<AppsPanel />)
    expect(await screen.findByText('No apps in Shop yet')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /New app/ })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show all projects' }))
    expect(getProjectScope()).toBe('')
  })

  it('keeps the plain message when no scope is picked', async () => {
    stubApi()
    renderUi(<AppsPanel />)
    expect(await screen.findByText('No apps match.')).toBeInTheDocument()
    expect(screen.queryByText(/No apps in/)).not.toBeInTheDocument()
  })

  it('says why the deploy feed is empty in a scope', async () => {
    stubApi()
    setProjectScope('p1')
    renderUi(<RecentActivity sample={[]} />)
    expect(
      await screen.findByText('No deploys yet for the apps in this scope.'),
    ).toBeInTheDocument()
  })
})

describe('no duplicated attention', () => {
  beforeEach(() => {
    attention.summary = {
      level: 'warning',
      count: 2,
      visible: [
        {
          id: 'login_approval:1',
          severity: 'warning',
          title: 'Sign-in waiting',
          detail: '',
          target: { kind: 'path', to: '/approve-sign-in' },
        },
        {
          id: 'app:web',
          severity: 'warning',
          title: 'web is failing',
          detail: '',
          target: { kind: 'app', name: 'web' },
        },
      ],
      snoozed: [],
      recommendations: [],
      snoozedRecommendations: [],
      isLoading: false,
    }
  })

  afterEach(() => {
    attention.summary = {
      level: 'ok',
      count: 0,
      visible: [],
      snoozed: [],
      recommendations: [],
      snoozedRecommendations: [],
      isLoading: false,
    }
  })

  it('leaves sign-in requests to the banner and renders one strip', async () => {
    stubApi()
    const { container } = renderUi(<StatusStrip apps={1} />)
    expect(await screen.findByText('1 thing needs attention')).toBeVisible()
    expect(container.querySelectorAll('[role="status"]')).toHaveLength(1)
  })
})
