import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { DomainIssueFix } from './DomainIssueFix'
import domainsEn from '../locales/en/domains.json'
import type { Domain, DomainIssue } from '../queries/domains'

const isRoot = vi.hoisted(() => ({ value: true }))

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

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => isRoot.value }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const domain = {
  domain: 'app.example.com',
  service_name: 'web',
  kind: 'app',
} as unknown as Domain

function issue(code: string): DomainIssue {
  return {
    domain: 'app.example.com',
    code,
    severity: 'degraded',
    message: code,
  }
}

function renderFix(code: string, d: Domain = domain) {
  const client = new QueryClient()
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <DomainIssueFix domain={d} issue={issue(code)} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('DomainIssueFix', () => {
  it('offers the automatic certificate fix to root for an unresolved custom cert', () => {
    isRoot.value = true
    renderFix('custom_cert_unresolved')
    expect(
      screen.getByRole('button', { name: 'Use automatic certificate' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Upload again')).toBeInTheDocument()
  })

  it('hides the automatic fix from a non-root operator but keeps the guided one', () => {
    isRoot.value = false
    renderFix('custom_cert_unresolved')
    expect(
      screen.queryByRole('button', { name: 'Use automatic certificate' }),
    ).not.toBeInTheDocument()
    expect(screen.getByText('Upload again')).toBeInTheDocument()
  })

  it.each([
    ['basic_auth_unresolved', 'Set password'],
    ['dns01_unavailable', 'Connect DNS provider'],
    ['target_unavailable', 'Open app'],
  ])('gives %s a "%s" fix', (code, label) => {
    renderFix(code)
    expect(screen.getByText(label)).toBeInTheDocument()
  })

  it('renders nothing for an unknown issue code', () => {
    renderFix('something_new')
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
