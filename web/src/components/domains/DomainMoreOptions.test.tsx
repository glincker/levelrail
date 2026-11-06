import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import domainsEn from '../../locales/en/domains.json'
import { DomainMoreOptions } from './DomainMoreOptions'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    ...rest
  }: { children?: ReactNode } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href="/x" {...rest}>
      {children}
    </a>
  ),
}))
vi.mock('../../queries/appNetwork', () => ({
  useAppNetwork: () => ({ data: { running: false } }),
}))
vi.mock('../../queries/domainBasicAuth', () => ({
  useDomainBasicAuth: () => ({ data: { enabled: true } }),
}))
vi.mock('../../queries/domainAuthGate', () => ({
  useDomainAuthGate: () => ({ data: { enabled: false } }),
}))
vi.mock('../../queries/domainRedirect', () => ({
  useDomainRedirect: () => ({ data: { enabled: false } }),
}))
vi.mock('../../queries/domainMaintenance', () => ({
  useDomainMaintenance: () => ({ data: { enabled: false } }),
}))
vi.mock('../../queries/domainWaf', () => ({
  useDomainWaf: () => ({
    data: { waf_enabled: false, rate_limit_enabled: false },
  }),
}))
vi.mock('../../queries/domainErrorPages', () => ({
  useDomainErrorPages: () => ({ data: { pages: [] } }),
}))
vi.mock('../../queries/domainSearchVisibility', () => ({
  useDomainSearchVisibility: () => ({ data: { hidden: true } }),
}))
vi.mock('../../queries/domainTlsCert', () => ({
  useDomainTLSCert: () => ({ data: { enabled: false } }),
}))
vi.mock('../DomainBasicAuthControl', () => ({
  DomainBasicAuthControl: () => <div data-testid="basic-auth" />,
}))
vi.mock('../DomainAuthGateControl', () => ({
  DomainAuthGateControl: () => null,
}))
vi.mock('../DomainDnsRecordsControl', () => ({
  DomainDnsRecordsControl: () => null,
}))
vi.mock('../DomainRedirectControl', () => ({
  DomainRedirectControl: () => null,
}))
vi.mock('../DomainMaintenanceControl', () => ({
  DomainMaintenanceControl: () => null,
}))
vi.mock('../DomainWafControl', () => ({ DomainWafControl: () => null }))
vi.mock('../DomainErrorPagesControl', () => ({
  DomainErrorPagesControl: () => null,
}))
vi.mock('../DomainSearchVisibilityControl', () => ({
  DomainSearchVisibilityControl: () => null,
}))
vi.mock('../DomainTLSCertControl', () => ({ DomainTLSCertControl: () => null }))
vi.mock('./DomainDiagnoseDialog', () => ({ DomainDiagnoseDialog: () => null }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

describe('DomainMoreOptions', () => {
  it('is collapsed, shows chips only for options that are on', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <DomainMoreOptions appName="web" domain="a.com" />
      </I18nextProvider>,
    )
    const toggle = screen.getByRole('button', { name: 'More options' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('Basic auth')).toBeInTheDocument()
    expect(screen.getByText('Hidden from search')).toBeInTheDocument()
    expect(screen.queryByText('Maintenance')).toBeNull()
    expect(screen.queryByTestId('basic-auth')).toBeNull()
    expect(
      screen.getByText('App is not running, see deploys'),
    ).toBeInTheDocument()
  })

  it('opens the three groups on click', async () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <DomainMoreOptions appName="web" domain="a.com" />
      </I18nextProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'More options' }))
    expect(screen.getByTestId('basic-auth')).toBeInTheDocument()
    for (const g of ['Access', 'Routing', 'Protection and polish']) {
      expect(screen.getByText(g)).toBeInTheDocument()
    }
  })
})
