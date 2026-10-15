import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import maintenanceEn from '../../locales/en/maintenance.json'
import { DomainMaintenanceBanner } from './DomainMaintenanceBanner'

const domains = vi.hoisted(() => ({
  data: [] as {
    domain: string
    service_name: string
    maintenance_enabled: boolean
  }[],
}))

vi.mock('../../queries/domains', () => ({
  domainKeys: { all: ['domains'] },
  domainsQueryOptions: () => ({
    queryKey: ['domains'],
    queryFn: () => Promise.resolve(domains.data),
  }),
}))
vi.mock('../../queries/domainMaintenance', () => ({
  useClearDomainMaintenance: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="#d">{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['maintenance'],
  defaultNS: 'maintenance',
  resources: { en: { maintenance: maintenanceEn } },
  interpolation: { escapeValue: false },
})

function renderBanner() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={qc}>
        <DomainMaintenanceBanner />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('DomainMaintenanceBanner', () => {
  it('renders nothing when no domain is in maintenance', () => {
    domains.data = [
      { domain: 'a.test', service_name: 'a', maintenance_enabled: false },
    ]
    const { container } = renderBanner()
    expect(container).toBeEmptyDOMElement()
  })

  it('shows a sticky notice with an end action for a domain in maintenance', async () => {
    domains.data = [
      { domain: 'down.test', service_name: 'down', maintenance_enabled: true },
      { domain: 'up.test', service_name: 'up', maintenance_enabled: false },
    ]
    renderBanner()
    expect(
      await screen.findByText('Maintenance mode is on for down.test'),
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'End maintenance' }),
    ).toBeEnabled()
    expect(screen.queryByText(/up\.test/)).toBeNull()
  })
})
