import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainsTable } from './DomainsTable'
import domainsEn from '../locales/en/domains.json'
import uiSharedEn from '../locales/en/uiShared.json'
import type { Domain } from '../queries/domains'
import { createTestQueryClient } from '@/test/testQueryClient'

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
  ns: ['domains', 'uiShared'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn, uiShared: uiSharedEn } },
  interpolation: { escapeValue: false },
})

function makeDomain(over: Partial<Domain> = {}): Domain {
  return {
    domain: 'app.example.com',
    service_name: 'web',
    waf_enabled: false,
    has_redirect: false,
    maintenance_enabled: false,
    has_basic_auth: false,
    ...over,
  }
}

function renderTable(props: {
  domains: Domain[]
  appCount: number
  onAdd?: () => void
}) {
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <DomainsTable
          domains={props.domains}
          certByDomain={new Map()}
          appCount={props.appCount}
          onAdd={props.onAdd ?? (() => undefined)}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('DomainsTable', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('points to deploying an app when there are no apps', () => {
    renderTable({ domains: [], appCount: 0 })
    expect(screen.getByText('No apps to route yet')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Deploy an app' })).toHaveAttribute(
      'href',
      '/apps',
    )
  })

  it('offers Add domain when apps exist but no domains do', async () => {
    const onAdd = vi.fn()
    const user = userEvent.setup()
    renderTable({ domains: [], appCount: 2, onAdd })
    expect(screen.getByText('No domains yet')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Add domain' }))
    expect(onAdd).toHaveBeenCalledTimes(1)
  })

  it('shows a filter empty state with a way back', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>(() => undefined)),
    )
    const user = userEvent.setup()
    renderTable({ domains: [makeDomain()], appCount: 1 })
    await user.type(screen.getByLabelText('Filter domains'), 'zzz')
    expect(screen.getByText('Search: zzz')).toBeInTheDocument()
    expect(
      screen.getByText('Nothing matches these filters'),
    ).toBeInTheDocument()
    await user.click(
      screen.getAllByRole('button', { name: 'Clear all filters' })[0]!,
    )
    expect(screen.getByLabelText('Filter domains')).toHaveValue('')
    expect(screen.queryByText('Search: zzz')).not.toBeInTheDocument()
  })
})

describe('DomainsTable environment and TLS', () => {
  it('shows the environment and a tone-marked TLS days left', () => {
    renderTable({
      domains: [
        makeDomain({
          environment: 'staging',
          environment_id: 'env_1',
          tls_days_left: -2,
        }),
      ],
      appCount: 1,
    })
    expect(screen.getAllByText('staging').length).toBeGreaterThan(0)
    const days = screen.getAllByText('Expired 2 days ago')[0]
    expect(days?.className).toContain('text-tone-danger')
  })
})
