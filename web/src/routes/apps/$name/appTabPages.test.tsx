import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  app: { name: 'web', domains: [] as string[] },
  source: { data: undefined as undefined | { preview_enabled: boolean } },
  previews: { data: [] as unknown[] },
  attempts: { isLoading: false, data: [] as unknown[] },
  conditions: [] as unknown[],
}))

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => (opts: object) => ({
    options: opts,
    useParams: () => ({ name: 'web' }),
    useSearch: () => ({}),
    useNavigate: () => vi.fn(),
  }),
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/d">{children}</a>
  ),
}))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => state.attempts,
}))
vi.mock('../../../queries/deploys', () => ({
  useDeployStatus: () => ({ data: state.conditions }),
}))
vi.mock('../../../queries/deployAttempts', () => ({
  deployAttemptsQueryOptions: vi.fn(),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@thesvg/ui-react', () => ({
  GlobeGlassIcon: () => null,
  ArchiveGlassIcon: () => null,
  BugGlassIcon: () => null,
  SearchGlassIcon: () => null,
  TerminalGlassIcon: () => null,
}))
vi.mock('../../../components/LogSearchPanel', () => ({
  LogSearchPanel: () => <div>LogSearchPanel</div>,
}))
vi.mock('../../../components/LiveLogViewer', () => ({
  LiveLogViewer: () => <div>LiveLogViewer</div>,
}))
vi.mock('../../../components/LogArchivePanel', () => ({
  LogArchivePanel: () => <div>LogArchivePanel</div>,
}))
vi.mock('../../../components/LogSignaturesPanel', () => ({
  LogSignaturesPanel: () => <div>LogSignaturesPanel</div>,
}))
vi.mock('../../../components/CreateLogAlertDialog', () => ({
  CreateLogAlertDialog: () => <div>CreateLogAlertDialog</div>,
}))
vi.mock('../../../components/LogsServiceSelect', () => ({
  LogsServiceSelect: () => <div>LogsServiceSelect</div>,
}))
vi.mock('../../../components/SavedLogQueriesMenu', () => ({
  SavedLogQueriesMenu: () => <div>SavedLogQueriesMenu</div>,
}))
vi.mock('../../../components/logBudgets/AppLogBudgetField', () => ({
  AppLogBudgetField: () => null,
}))
vi.mock('../../../queries/apps', () => ({
  useApp: () => ({ data: state.app }),
}))
vi.mock('../../../queries/certificates', () => ({
  certificatesQueryOptions: vi.fn(),
}))
vi.mock('../../../queries/gitSources', () => ({
  useGitSource: () => ({ isLoading: false, data: state.source.data }),
}))
vi.mock('../../../queries/previewEnvironments', () => ({
  usePreviewEnvironments: () => ({
    isLoading: false,
    data: state.previews.data,
  }),
}))
vi.mock('../../../lib/gitProviderSupport', () => ({
  supportsPrFeatures: () => true,
}))
vi.mock('../../../components/maintenance/AppMaintenanceToggle', () => ({
  AppMaintenanceToggle: () => null,
}))
vi.mock('../../../components/DomainEditor', () => ({
  DomainEditor: () => <div>domain-editor</div>,
}))
vi.mock('../../../components/AppIngressLimitsCard', () => ({
  AppIngressLimitsCard: () => <div>ingress-limits</div>,
}))
vi.mock('../../../components/ShieldRecommendedCard', () => ({
  ShieldRecommendedCard: () => <div>shield</div>,
}))
vi.mock('../../../components/PreviewEnvironmentsCard', () => ({
  PreviewEnvironmentsCard: () => <div>preview-envs</div>,
}))
vi.mock('../../../components/PreviewTtlCard', () => ({
  PreviewTtlCard: () => <div>ttl</div>,
}))
vi.mock('../../../components/PreviewPolicyCard', () => ({
  PreviewPolicyCard: () => <div>policy</div>,
}))
vi.mock('../../../components/BranchDeploysCard', () => ({
  BranchDeploysCard: () => <div>branch-deploys</div>,
}))
vi.mock('../../../components/DeployContextsCard', () => ({
  DeployContextsCard: () => <div>contexts</div>,
}))
vi.mock('../../../components/PreviewLifecycleCard', () => ({
  PreviewLifecycleCard: () => <div>lifecycle</div>,
}))

import { Route as DomainsRoute } from './domains'
import { Route as PreviewsRoute } from './previews'
import { Route as LogsRoute } from './logs'

function renderRoute(route: { options: { component?: unknown } }) {
  const Page = route.options.component as () => React.ReactNode
  render(<Page />)
}

beforeEach(() => {
  state.app = { name: 'web', domains: [] }
  state.source.data = undefined
  state.previews.data = []
  state.attempts = { isLoading: false, data: [] }
  state.conditions = []
})

describe('domains tab', () => {
  it('hides the shield and ingress cards with no domain', () => {
    renderRoute(DomainsRoute)
    expect(screen.getByText('domain-editor')).toBeInTheDocument()
    expect(screen.queryByText('shield')).not.toBeInTheDocument()
    expect(screen.queryByText('ingress-limits')).not.toBeInTheDocument()
  })
  it('shows them once a domain exists', () => {
    state.app = { name: 'web', domains: ['a.example.com'] }
    renderRoute(DomainsRoute)
    expect(screen.getByText('shield')).toBeInTheDocument()
    expect(screen.getByText('ingress-limits')).toBeInTheDocument()
  })
})

describe('previews tab', () => {
  it('shows only previews and branch deploys when off and empty', () => {
    renderRoute(PreviewsRoute)
    expect(screen.getByText('preview-envs')).toBeInTheDocument()
    expect(screen.getByText('branch-deploys')).toBeInTheDocument()
    for (const x of ['ttl', 'contexts', 'policy', 'lifecycle']) {
      expect(screen.queryByText(x)).not.toBeInTheDocument()
    }
  })
  it('shows the rest once PR previews are on', () => {
    state.source.data = { preview_enabled: true }
    renderRoute(PreviewsRoute)
    for (const x of ['ttl', 'contexts', 'policy', 'lifecycle']) {
      expect(screen.getByText(x)).toBeInTheDocument()
    }
  })
})

describe('logs tab', () => {
  it('shows one empty state and no tabs before the first deploy', () => {
    renderRoute(LogsRoute)
    expect(screen.getByText('liveLogs.neverDeployedTitle')).toBeInTheDocument()
    expect(screen.getByText('liveLogs.neverDeployedAction')).toBeInTheDocument()
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
  })
  it('shows nothing while loading', () => {
    state.attempts = { isLoading: true, data: [] }
    renderRoute(LogsRoute)
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    expect(
      screen.queryByText('liveLogs.neverDeployedTitle'),
    ).not.toBeInTheDocument()
  })
  it('shows the tabs once the app has deployed', () => {
    state.attempts = { isLoading: false, data: [{ id: 1 }] }
    renderRoute(LogsRoute)
    expect(screen.getByRole('tablist')).toBeInTheDocument()
    expect(
      screen.queryByText('liveLogs.neverDeployedTitle'),
    ).not.toBeInTheDocument()
  })
})
