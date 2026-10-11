import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TemplateMarketplace } from './TemplateMarketplace'
import { TemplateStackPanel } from './TemplateStackPanel'
import composeEn from '../locales/en/compose.json'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['compose'],
  defaultNS: 'compose',
  resources: { en: { compose: composeEn } },
  interpolation: { escapeValue: false },
})

const catalog = [
  {
    id: 'gitea-with-postgresql',
    name: 'Gitea With Postgresql',
    slogan: 'Git hosting',
    category: 'Developer Tools',
    documentation_url: 'https://docs.gitea.com',
    requires_configuration: true,
    installable: true,
  },
  {
    id: 'legacy-app',
    name: 'Legacy App',
    slogan: 'Older single body entry',
    category: 'Developer Tools',
    documentation_url: 'https://example.com',
    requires_configuration: false,
  },
  {
    id: 'grafana',
    name: 'Grafana',
    slogan: 'Dashboards',
    category: 'Monitoring',
    documentation_url: 'https://grafana.com',
    requires_configuration: false,
  },
]

function stubFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const body = url.includes('/templates/custom') ? [] : catalog
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
        text: () => Promise.resolve(JSON.stringify(body)),
        headers: new Headers({ 'Content-Type': 'application/json' }),
      } as unknown as Response)
    }),
  )
}

function renderMarketplace() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <TemplateMarketplace onSelect={vi.fn()} onDeployNow={vi.fn()} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('TemplateMarketplace', () => {
  it('narrows the catalog by search, category and install form', async () => {
    stubFetch()
    renderMarketplace()
    await screen.findByText('3 of 3 templates')

    await userEvent.type(screen.getByLabelText('Search templates'), 'gitea')
    await screen.findByText('1 of 3 templates')

    await userEvent.clear(screen.getByLabelText('Search templates'))
    await userEvent.click(
      screen.getByRole('button', { name: 'Install form only' }),
    )
    await screen.findByText('1 of 3 templates')

    await userEvent.click(
      screen.getByRole('button', { name: 'Install form only' }),
    )
    await userEvent.click(
      screen.getAllByRole('button', { name: /Monitoring/ })[0] as HTMLElement,
    )
    await waitFor(() => {
      expect(screen.getByText('1 of 3 templates')).toBeTruthy()
    })
  })

  it('offers a way out when nothing matches', async () => {
    stubFetch()
    renderMarketplace()
    await screen.findByText('3 of 3 templates')
    await userEvent.type(screen.getByLabelText('Search templates'), 'zzzz')
    await screen.findByText('No templates match')
    await userEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    await screen.findByText('3 of 3 templates')
  })
})

describe('TemplateStackPanel', () => {
  it('lists services with images, ports and volumes and the source', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <TemplateStackPanel
          stack={[
            {
              name: 'gitea',
              image: 'gitea/gitea:latest',
              ports: [3000],
              volumes: ['gitea-data:/data'],
            },
            { name: 'postgresql', image: 'postgres:16-alpine' },
          ]}
          source={{
            name: 'coolify',
            url: 'https://github.com/coollabsio/coolify',
            license: 'Apache-2.0',
            path: 'templates/compose/gitea-with-postgresql.yaml',
          }}
        />
      </I18nextProvider>,
    )
    expect(screen.getByText('gitea/gitea:latest')).toBeTruthy()
    expect(screen.getByText('3000')).toBeTruthy()
    expect(screen.getByText('gitea-data:/data')).toBeTruthy()
    expect(screen.getByText(/licence Apache-2.0/)).toBeTruthy()
  })
})
