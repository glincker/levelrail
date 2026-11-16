import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  app: { name: 'web', domains: [] as string[] },
  source: { data: undefined as undefined | { preview_enabled: boolean } },
  previews: { data: [] as unknown[] },
}))

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => (opts: object) => ({
    options: opts,
    useParams: () => ({ name: 'web' }),
  }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('@thesvg/ui-react', () => ({ GlobeGlassIcon: () => null }))
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

function renderRoute(route: { options: { component?: unknown } }) {
  const Page = route.options.component as () => React.ReactNode
  render(<Page />)
}

beforeEach(() => {
  state.app = { name: 'web', domains: [] }
  state.source.data = undefined
  state.previews.data = []
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
