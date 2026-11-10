import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import domains from '../../locales/en/domains.json'
import { DomainDetailPage } from './DomainDetailPage'

interface AbilityState {
  role: string | null
  can: (ability: string) => boolean
}

const abilities = vi.hoisted((): { value: AbilityState } => ({
  value: { role: null, can: () => false },
}))

vi.mock('../../hooks/useAbilities', () => ({
  useAbilities: () => abilities.value,
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))
vi.mock('@tanstack/react-query', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({ data: undefined }),
}))
vi.mock('./useDomainCheckState', () => ({
  useDomainCheckState: () => ({
    data: undefined,
    isFetching: false,
    onRefetch: () => undefined,
    proxyRow: undefined,
  }),
}))
vi.mock('./useRecheckCountdown', () => ({ useRecheckCountdown: () => 0 }))
vi.mock('../domain-tuning/DomainTuningSection', () => ({
  default: () => null,
}))
vi.mock('./DomainTrafficLimitsControl', () => ({
  DomainTrafficLimitsControl: () => <button type="button">Save limits</button>,
}))
vi.mock('../DomainAuthGateControl', () => ({
  DomainAuthGateControl: () => null,
}))
vi.mock('../DomainBasicAuthControl', () => ({
  DomainBasicAuthControl: () => null,
}))
vi.mock('../DomainDnsRecordsControl', () => ({
  DomainDnsRecordsControl: () => null,
}))
vi.mock('../DomainErrorPagesControl', () => ({
  DomainErrorPagesControl: () => null,
}))
vi.mock('../DomainMaintenanceControl', () => ({
  DomainMaintenanceControl: () => null,
}))
vi.mock('../DomainRedirectControl', () => ({
  DomainRedirectControl: () => null,
}))
vi.mock('../DomainSearchVisibilityControl', () => ({
  DomainSearchVisibilityControl: () => null,
}))
vi.mock('../DomainTLSCertControl', () => ({ DomainTLSCertControl: () => null }))
vi.mock('../DomainWafControl', () => ({ DomainWafControl: () => null }))
vi.mock('../ProxyDomainStatus', () => ({ ProxyDomainStatus: () => null }))
vi.mock('./DomainRollbackStrip', () => ({ DomainRollbackStrip: () => null }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { domains } },
  interpolation: { escapeValue: false },
})

function renderPage() {
  render(
    <I18nextProvider i18n={testI18n}>
      <DomainDetailPage appName="demo" domain="a.example.com" />
    </I18nextProvider>,
  )
}

describe('DomainDetailPage read-only gate', () => {
  beforeEach(() => {
    abilities.value = { role: null, can: () => false }
  })

  it('disables controls and explains it for a viewer', () => {
    abilities.value = { role: 'viewer', can: (a) => a === 'read' }
    renderPage()
    expect(screen.getByRole('button', { name: 'Save limits' })).toBeDisabled()
    expect(screen.getByText(domains.readOnlyNotice)).toBeInTheDocument()
  })

  it('keeps controls enabled and hides the notice for an operator', () => {
    abilities.value = { role: 'operator', can: () => true }
    renderPage()
    expect(screen.getByRole('button', { name: 'Save limits' })).toBeEnabled()
    expect(screen.queryByText(domains.readOnlyNotice)).toBeNull()
  })

  it('stays disabled without a notice while the role is unknown', () => {
    renderPage()
    expect(screen.getByRole('button', { name: 'Save limits' })).toBeDisabled()
    expect(screen.queryByText(domains.readOnlyNotice)).toBeNull()
  })
})
