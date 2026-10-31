import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import maintenanceEn from '../../locales/en/maintenance.json'
import { DomainMaintenanceBanner } from './DomainMaintenanceBanner'
import { createTestQueryClient } from '@/test/testQueryClient'

const domains = vi.hoisted(() => ({
  data: [] as {
    domain: string
    service_name: string
    maintenance_enabled: boolean
    enforcement?: { mode: string; maintenance: boolean }
  }[],
}))

vi.mock('../../queries/domains', () => ({
  domainKeys: { all: ['domains'] },
  domainsQueryOptions: () => ({
    queryKey: ['domains'],
    queryFn: () => Promise.resolve(domains.data),
  }),
}))
const fleet = vi.hoisted(() => ({
  items: [] as Record<string, unknown>[],
}))
vi.mock('../../queries/domainMaintenance', () => ({
  useClearDomainMaintenance: () => ({ mutate: vi.fn(), isPending: false }),
  fleetMaintenanceQueryOptions: () => ({
    queryKey: ['fleet'],
    queryFn: () =>
      Promise.resolve({
        items: fleet.items,
        max_duration_seconds: 0,
        attention_after_seconds: 3600,
      }),
  }),
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
  const qc = createTestQueryClient()
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
      {
        domain: 'down.test',
        service_name: 'down',
        maintenance_enabled: true,
        enforcement: { mode: 'levelrail', maintenance: true },
      },
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

  it('never says maintenance is on when the proxy path does not enforce it', async () => {
    domains.data = [
      {
        domain: 'proxied.test',
        service_name: 'web',
        maintenance_enabled: true,
        enforcement: { mode: 'proxy:traefik', maintenance: false },
      },
    ]
    renderBanner()
    expect(
      await screen.findByText(
        'Maintenance is saved for proxied.test but not enforced',
      ),
    ).toBeVisible()
    expect(screen.queryByText(/Maintenance mode is on/)).toBeNull()
  })

  it('shows how long maintenance has been on and flags a long run', async () => {
    domains.data = [
      {
        domain: 'down.test',
        service_name: 'down',
        maintenance_enabled: true,
        enforcement: { mode: 'levelrail', maintenance: true },
      },
    ]
    fleet.items = [
      {
        domain: 'down.test',
        app: 'down',
        enabled: true,
        elapsed_seconds: 0,
        started_at: new Date(Date.now() - 3 * 3600 * 1000).toISOString(),
      },
    ]
    renderBanner()
    expect(await screen.findByText(/On for 3h 0m\./)).toBeVisible()
    expect(screen.getByText(/No automatic end/)).toBeVisible()
    expect(screen.getByText(/longer than expected/)).toBeVisible()
    fleet.items = []
  })
})
