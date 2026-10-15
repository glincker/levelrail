import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ComponentType } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import betaEn from '../../locales/en/betaFeatures.json'
import advisorEn from '../../locales/en/resourceAdvisor.json'
import commonEn from '../../locales/en/common.json'
import { bannerFor, groupByMaturity } from '../../lib/betaFeatures'
import type { BetaFeature } from '../../types/resourceAdvisor'
import { Route } from './beta-features'

const push = vi.fn()
const scrollIntoView = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useRouter: () => ({ history: { push } }) }
})
vi.mock('@/components/HelpLink', () => ({ HelpLink: () => null }))
vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

function feature(over: Partial<BetaFeature>): BetaFeature {
  return {
    id: 'x',
    name: 'X',
    summary: 'About x',
    enabled: false,
    enabled_by: 'none',
    toggleable: false,
    requires_restart: true,
    env_hint: '',
    docs_path: '/x',
    maturity: 'experimental',
    locked: false,
    ...over,
  }
}

let list: BetaFeature[] = []
const put = vi.fn((id: string, enabled: boolean) => {
  list = list.map((f) => (f.id === id ? { ...f, enabled } : f))
  return Promise.resolve(list.find((f) => f.id === id))
})

beforeEach(() => {
  push.mockClear()
  put.mockClear()
  list = [
    feature({
      id: 'resource-advisor',
      name: 'Resource advisor',
      maturity: 'beta',
      toggleable: true,
      requires_restart: false,
    }),
    feature({ id: 'load-balancer', name: 'Load balancer' }),
  ]
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
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
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
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

  it('shows the exact env line for a startup-only feature', async () => {
    renderPage({ feature: 'load-balancer', from: '/loadbalancers' })
    expect(
      await screen.findByText('APP_EXPERIMENTAL=load-balancer'),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/needs a restart after you change the setting/),
    ).toBeInTheDocument()
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
