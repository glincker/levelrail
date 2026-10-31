import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ComponentType } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import betaEn from '../../locales/en/betaFeatures.json'
import advisorEn from '../../locales/en/resourceAdvisor.json'
import commonEn from '../../locales/en/common.json'
import { bannerFor, groupByMaturity } from '../../lib/betaFeatures'
import type { BetaFeature } from '../../types/resourceAdvisor'
import type { ServerProfile } from '../../queries/serverProfile'
import { Route } from './beta-features'
import { createTestQueryClient } from '@/test/testQueryClient'

const push = vi.fn()
const scrollIntoView = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useRouter: () => ({ history: { push } }) }
})
vi.mock('@/components/HelpLink', () => ({ HelpLink: () => null }))
vi.mock('@/hooks/useBrand', () => ({
  useBrand: () => ({ BinaryName: 'tool', ShortName: 'tool' }),
}))
vi.mock('@/hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

function feature(over: Partial<BetaFeature>): BetaFeature {
  return {
    id: 'x',
    name: 'X',
    summary: 'About x',
    enabled: false,
    enabled_by: 'none',
    toggleable: true,
    applies_live: false,
    pending_restart: false,
    requires_restart: true,
    env_hint: '',
    docs_path: '/x',
    maturity: 'experimental',
    locked: false,
    ...over,
  }
}

const profileInfo = (id: ServerProfile['configured']) => ({
  id,
  mesh: id === 'suite',
  plugins: id === 'suite',
  experimental_features: id === 'suite' ? 13 : 0,
  retention_days: id === 'minimal' ? 3 : 15,
  loopback_agent: id === 'minimal',
  lazy_loops: id === 'minimal',
  posture_recommendations: id === 'minimal',
  sbom_scan: id === 'suite',
})
let serverProfile: ServerProfile = {
  active: 'standard',
  configured: 'standard',
  source: 'default',
  env_var: 'APP_SERVER_PROFILE',
  pinned_by_env: false,
  restart_required: false,
  profiles: [
    profileInfo('minimal'),
    profileInfo('standard'),
    profileInfo('suite'),
  ],
  problems: [],
  needs_confirm: false,
  saved: false,
}
const putProfile = vi.fn()
let list: BetaFeature[] = []
const put = vi.fn((id: string, enabled: boolean) => {
  list = list.map((f) => (f.id === id ? { ...f, enabled } : f))
  return Promise.resolve(list.find((f) => f.id === id))
})

beforeEach(() => {
  push.mockClear()
  put.mockClear()
  putProfile.mockReset()
  serverProfile = {
    ...serverProfile,
    configured: 'standard',
    pinned_by_env: false,
    restart_required: false,
  }
  list = [
    feature({
      id: 'resource-advisor',
      name: 'Resource advisor',
      maturity: 'beta',
      applies_live: true,
      requires_restart: false,
    }),
    feature({ id: 'load-balancer', name: 'Load balancer' }),
  ]
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      if (url.includes('server-profile')) {
        if (init?.method === 'PUT') {
          return Promise.resolve(putProfile(JSON.parse(init.body as string)))
        }
        return Promise.resolve(
          new Response(JSON.stringify(serverProfile), { status: 200 }),
        )
      }
      if (init?.method === 'PUT') {
        const id = decodeURIComponent(url.split('/').pop() ?? '')
        const body = JSON.parse(init.body as string) as { enabled: boolean }
        return put(id, body.enabled).then(
          (f) => new Response(JSON.stringify(f), { status: 200 }),
        )
      }
      return Promise.resolve(
        new Response(JSON.stringify(list), { status: 200 }),
      )
    }),
  )
  scrollIntoView.mockClear()
  Element.prototype.scrollIntoView = scrollIntoView
})

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['betaFeatures', 'resourceAdvisor', 'common'],
  defaultNS: 'common',
  resources: {
    en: { betaFeatures: betaEn, resourceAdvisor: advisorEn, common: commonEn },
  },
  interpolation: { escapeValue: false },
})

function renderPage(search: { feature?: string; from?: string }) {
  vi.spyOn(Route, 'useSearch').mockReturnValue(search)
  const Page = Route.options.component as ComponentType
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <Page />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('betaFeatures helpers', () => {
  it('groups beta before experimental and drops empty groups', () => {
    const groups = groupByMaturity([
      feature({ id: 'a', maturity: 'experimental' }),
      feature({ id: 'b', maturity: 'beta' }),
    ])
    expect(groups.map((g) => g.maturity)).toEqual(['beta', 'experimental'])
    expect(groupByMaturity([feature({ maturity: 'beta' })])).toHaveLength(1)
  })

  it.each([
    [undefined, 'none'],
    ['missing', 'unknown'],
    ['load-balancer', 'locked'],
    ['resource-advisor', 'ready'],
  ])('banner for %s is %s', (id, kind) => {
    const features = [
      feature({ id: 'load-balancer' }),
      feature({ id: 'resource-advisor', enabled: true }),
    ]
    expect(bannerFor(features, id).kind).toBe(kind)
  })
})

describe('Beta features page', () => {
  it('lists every feature grouped by maturity and highlights the requested one', async () => {
    renderPage({ feature: 'load-balancer' })
    const card = await screen.findByText('Load balancer')
    expect(screen.getByRole('heading', { name: 'Beta' })).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Experimental' }),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(document.getElementById('feature-load-balancer')).toHaveAttribute(
        'data-highlighted',
        'true',
      )
    })
    expect(card).toBeInTheDocument()
    expect(scrollIntoView).toHaveBeenCalled()
  })

  it('gives a startup-only feature a switch and keeps the env line secondary', async () => {
    renderPage({ feature: 'load-balancer', from: '/loadbalancers' })
    expect(
      await screen.findByRole('switch', { name: 'Turn on Load balancer' }),
    ).toBeInTheDocument()
    const openers = screen.getAllByRole('button', {
      name: 'Use the CLI or server config instead',
    })
    await userEvent.setup().click(openers[openers.length - 1]!)
    expect(
      await screen.findByText(
        'tool-cli settings beta-features set load-balancer on',
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByText('Environment=APP_EXPERIMENTAL=load-balancer', {
        exact: false,
      }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Turn it on from the server')).toBeNull()
    expect(
      screen.getByText('Load balancer is a beta feature'),
    ).toBeInTheDocument()
  })

  it('enables a toggleable feature then offers a way back', async () => {
    const user = userEvent.setup()
    renderPage({ feature: 'resource-advisor', from: '/resource-advisor' })
    expect(
      await screen.findByText('Resource advisor is a beta feature'),
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole('switch', { name: 'Turn on Resource advisor' }),
    )
    expect(put).toHaveBeenCalledWith('resource-advisor', true)
    const back = await screen.findByRole('button', { name: 'Take me there' })
    await user.click(back)
    expect(push).toHaveBeenCalledWith('/resource-advisor')
  })

  it('never offers a return link to an off-site path', async () => {
    list = list.map((f) => ({ ...f, enabled: true }))
    renderPage({ feature: 'resource-advisor', from: '//evil.example' })
    await screen.findByText('Resource advisor')
    expect(screen.queryByRole('button', { name: 'Take me there' })).toBeNull()
  })
})

describe('Server profile card', () => {
  it('shows the current profile and what each includes', async () => {
    renderPage({})
    expect(await screen.findByText('Server profile')).toBeInTheDocument()
    expect(screen.getByText('Current')).toBeInTheDocument()
    expect(screen.getByText('Mesh on')).toBeInTheDocument()
    expect(
      screen.getByText('Agent listener on loopback until a node enrolls'),
    ).toBeInTheDocument()
  })

  it('saves a chosen profile and asks for a restart', async () => {
    putProfile.mockImplementation(
      (body: { profile: ServerProfile['configured'] }) => {
        serverProfile = {
          ...serverProfile,
          configured: body.profile,
          restart_required: true,
        }
        return new Response(JSON.stringify(serverProfile), { status: 200 })
      },
    )
    renderPage({})
    const buttons = await screen.findAllByRole('button', {
      name: 'Use this profile',
    })
    await userEvent.setup().click(buttons[buttons.length - 1]!)
    await waitFor(() => {
      expect(putProfile).toHaveBeenCalledWith({
        profile: 'suite',
        confirm: false,
      })
    })
    expect(
      await screen.findByText('Saved. Restart to apply this change.', {
        exact: false,
      }),
    ).toBeInTheDocument()
  })

  it('shows a refused move and offers confirm for a shorter retention', async () => {
    putProfile.mockImplementationOnce(
      () =>
        new Response(
          JSON.stringify({
            ...serverProfile,
            needs_confirm: true,
            problems: [
              {
                code: 'retention_shorter',
                message: 'Older samples go.',
                blocking: false,
              },
            ],
            error: 'Older samples go.',
          }),
          { status: 409 },
        ),
    )
    renderPage({})
    const buttons = await screen.findAllByRole('button', {
      name: 'Use this profile',
    })
    await userEvent.setup().click(buttons[0]!)
    expect(await screen.findByText('Older samples go.')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Confirm and apply' }),
    ).toBeInTheDocument()
  })

  it('hides the choices when the server pins the profile', async () => {
    serverProfile = { ...serverProfile, pinned_by_env: true, source: 'env' }
    renderPage({})
    expect(await screen.findByText('Set by the server')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Use this profile' }),
    ).toBeNull()
  })
})
