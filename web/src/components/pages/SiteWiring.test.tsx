import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SiteWiring } from './SiteWiring'

const h: {
  domains: string[]
  submissions: unknown[]
  routes: unknown
  settings: unknown
} = vi.hoisted(() => ({
  domains: [],
  submissions: [],
  routes: undefined,
  settings: undefined,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o ? `${k}:${JSON.stringify(o)}` : k,
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('./SiteHero', () => ({ StatusDot: () => <span>dot</span> }))
vi.mock('../../queries/apps', () => ({
  useApp: () => ({ data: { domains: h.domains } }),
}))
vi.mock('../../queries/certificates', () => ({
  useCertificates: () => ({
    data: [{ domain: 'a.example.com', status: 'healthy' }],
  }),
}))
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({ data: undefined }),
}))
vi.mock('../../queries/pagesCiTokens', () => ({
  useSiteDeployTokens: () => ({
    data: [{ id: '1' }, { id: '2', revoked_at: 'x' }],
  }),
}))
vi.mock('../../queries/pagesHook', () => ({
  useSiteHook: () => ({ data: { enabled: true } }),
}))
vi.mock('../../queries/pagesSubmissions', () => ({
  useSiteSubmissions: () => ({ data: h.submissions }),
}))
vi.mock('../../queries/pagesSites', () => ({
  usePagesSiteDetail: () => ({ data: undefined }),
  useSiteSettings: () => ({ data: h.settings }),
}))
vi.mock('../../queries/staticRoutes', () => ({
  useStaticRoutes: () => ({ data: h.routes }),
}))

describe('SiteWiring', () => {
  it('offers one action per empty group', () => {
    h.domains = []
    h.submissions = []
    h.routes = undefined
    h.settings = undefined
    const onOpenTab = vi.fn<(tab: string) => void>()
    render(<SiteWiring name="docs" onOpenTab={onOpenTab} />)
    fireEvent.click(screen.getByText('success.addDomain'))
    fireEvent.click(screen.getByText('overview.wiring.addForm'))
    fireEvent.click(screen.getByText('overview.wiring.addRule'))
    expect(onOpenTab.mock.calls.map((c) => c[0])).toEqual([
      'domains',
      'settings',
      'routes',
    ])
  })

  it('counts domains, forms and rules from the real data', () => {
    h.domains = ['a.example.com', 'b.example.com', 'c.example.com', 'd.com']
    h.submissions = [
      { form: 'contact', spam: false },
      { form: 'contact', spam: false },
      { form: 'news', spam: true },
    ]
    h.routes = {
      rules: [{ from: '/a', to: 'app://api/x', status: 200 }],
      headers: [{ path: '/*' }],
    }
    h.settings = {
      protected: true,
      clean_urls: null,
      spa_fallback: null,
      effective: { clean_urls: true },
    }
    render(<SiteWiring name="docs" onOpenTab={vi.fn()} />)
    expect(screen.getByText('overview.wiring.tls.healthy')).toBeTruthy()
    expect(screen.getByText('overview.wiring.more:{"count":1}')).toBeTruthy()
    expect(
      screen.getByText('overview.wiring.submissions:{"count":2}'),
    ).toBeTruthy()
    expect(
      screen.getByText(
        'overview.wiring.ruleSummary:{"rules":1,"headers":1,"proxied":1}',
      ),
    ).toBeTruthy()
    expect(
      screen.getByText('overview.wiring.tokensActive:{"count":1}'),
    ).toBeTruthy()
  })
})
