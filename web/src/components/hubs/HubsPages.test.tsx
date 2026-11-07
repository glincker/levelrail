import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import hubsEn from '../../locales/en/hubs.json'
import { SourcesHub, VariablesHub } from './HubsPages'

vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: ReactNode }) => <a href="#x">{children}</a>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'hubs'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, hubs: hubsEn } },
  interpolation: { escapeValue: false },
})

const SECRET = 'super-secret-value'

function stubFetch(routes: Record<string, unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      await Promise.resolve()
      return new Response(JSON.stringify(routes[url] ?? []), { status: 200 })
    }),
  )
}

function renderUi(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('hubs', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(cleanup)

  it('lists scopes with counts and never renders values', async () => {
    stubFetch({
      '/api/v1/organizations': [{ id: 'o1', name: 'Acme', created_at: '' }],
      '/api/v1/organizations/o1/env': { A: SECRET, B: SECRET },
      '/api/v1/projects': [{ id: 'p1', name: 'Web', created_at: '' }],
      '/api/v1/projects/p1/env': { K: SECRET },
    })
    renderUi(<VariablesHub />)
    expect(await screen.findByText('Acme')).toBeTruthy()
    expect(await screen.findByText('2 variables')).toBeTruthy()
    expect(await screen.findByText('1 variable')).toBeTruthy()
    expect(document.body.textContent).not.toContain(SECRET)
  })

  it('shows empty states with no scopes', async () => {
    stubFetch({})
    renderUi(<VariablesHub />)
    expect(await screen.findByText('No organizations')).toBeTruthy()
    expect(await screen.findByText('No projects')).toBeTruthy()
  })

  it('renders provider rows with status and account counts', async () => {
    stubFetch({
      '/api/v1/git-providers': [
        {
          provider: 'github',
          connected: true,
          connections: 2,
          connection: { status: 'ok' },
        },
        {
          provider: 'gitlab',
          connected: true,
          connections: 1,
          connection: { status: 'revoked' },
        },
        { provider: 'gitea', connected: false },
      ],
    })
    renderUi(<SourcesHub />)
    expect(await screen.findByText('GitHub')).toBeTruthy()
    expect(screen.getByText('2 accounts')).toBeTruthy()
    expect(screen.getByText('Revoked')).toBeTruthy()
    expect(screen.getByText('Not connected')).toBeTruthy()
  })

  it('shows an empty state with no providers', async () => {
    stubFetch({ '/api/v1/git-providers': [] })
    renderUi(<SourcesHub />)
    expect(await screen.findByText('No git providers')).toBeTruthy()
  })
})
