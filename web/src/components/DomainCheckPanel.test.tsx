import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { DomainCheckPanel, type DomainCheckPanelData } from './DomainDnsCheck'
import domainsEn from '../locales/en/domains.json'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const staleFailure = {
  error: 'tls: unrecognized name (ca=acme-staging)',
  renewal: false,
  at: '2026-10-10T00:00:00Z',
  reason: 'other_proxy',
  action: 'other_proxy' as const,
}

function renderPanel(data: DomainCheckPanelData) {
  render(
    <I18nextProvider i18n={testI18n}>
      <DomainCheckPanel
        domain="app.example.com"
        data={data}
        isFetching={false}
        onRefetch={() => undefined}
      />
    </I18nextProvider>,
  )
}

describe('DomainCheckPanel proxy-managed state', () => {
  it('shows ACME failure, DNS-01 provider and private-address guidance when this server owns TLS', () => {
    renderPanel({
      status: 'connected',
      expected_private: true,
      expected_host: '10.0.0.5',
      dns_provider: 'none',
      acme_failure: staleFailure,
    })
    expect(screen.getByText(/answers port 80/i)).toBeInTheDocument()
    expect(screen.queryByText(/managed by your proxy/i)).not.toBeInTheDocument()
  })

  it('replaces them with one proxy-managed line and a link to the proxy page', () => {
    renderPanel({
      status: 'connected',
      expected_private: true,
      expected_host: '10.0.0.5',
      dns_provider: 'none',
      acme_failure: staleFailure,
      tls_owner: 'proxy',
      proxy_certificate: {
        issuer: 'R11',
        not_after: '2027-01-05T00:00:00Z',
        status: 'healthy',
      },
    })
    expect(screen.queryByText(/answers port 80/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/certificate request failed/i)).toBeNull()
    expect(
      screen.getByText(/managed by your proxy \(issued by R11, expires/i),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /open the proxy page/i }),
    ).toHaveAttribute('href', '/network/proxy')
  })
})
