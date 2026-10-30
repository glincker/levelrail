import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DomainEditor } from './DomainEditor'
import domainsEn from '../locales/en/domains.json'
import type { AppDetail } from '../types/appDetail'

const updateMutate = vi.fn()
const updateState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

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

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

vi.mock('../queries/apps', () => ({
  useUpdateApp: () => ({ mutate: updateMutate, ...updateState }),
}))

vi.mock('../queries/certificates', () => ({
  useCertificates: () => ({ data: [] }),
}))

vi.mock('./DomainEnvironmentTabs', () => ({
  DomainEnvironmentTabs: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))
vi.mock('./domains/DomainStepper', () => ({
  DomainStepper: () => <div data-testid="wizard" />,
}))
vi.mock('./AutomaticDomainRow', () => ({ AutomaticDomainRow: () => null }))
vi.mock('./DomainDnsCheck', () => ({
  DomainDnsCheck: ({ domain }: { domain: string }) => (
    <div data-testid="dns-check">{domain}</div>
  ),
}))
vi.mock('./ProxyDomainStatus', () => ({
  ProxyDomainStatus: () => null,
  ProxyHandledTlsBadge: () => <span>proxy tls</span>,
}))
vi.mock('./DomainDnsRecordsControl', () => ({
  DomainDnsRecordsControl: () => null,
}))
vi.mock('./DomainBasicAuthControl', () => ({
  DomainBasicAuthControl: () => <div data-testid="basic-auth" />,
}))
vi.mock('./DomainAuthGateControl', () => ({
  DomainAuthGateControl: () => null,
}))
vi.mock('./DomainErrorPagesControl', () => ({
  DomainErrorPagesControl: () => null,
}))
vi.mock('./DomainMaintenanceControl', () => ({
  DomainMaintenanceControl: () => null,
}))
vi.mock('./DomainRedirectControl', () => ({
  DomainRedirectControl: () => null,
}))
vi.mock('./DomainSearchVisibilityControl', () => ({
  DomainSearchVisibilityControl: () => null,
}))
vi.mock('./DomainTLSCertControl', () => ({ DomainTLSCertControl: () => null }))
vi.mock('./DomainWafControl', () => ({ DomainWafControl: () => null }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function appWith(domains: string[]): AppDetail {
  return { name: 'web', port: 3000, domains } as unknown as AppDetail
}

function renderEditor(domains: string[] = []) {
  render(
    <I18nextProvider i18n={testI18n}>
      <DomainEditor app={appWith(domains)} />
    </I18nextProvider>,
  )
}

beforeEach(() => {
  updateMutate.mockReset()
  updateState.isPending = false
  updateState.isError = false
  updateState.error = null
})

describe('DomainEditor', () => {
  it('shows an empty state when the app has no domains', () => {
    renderEditor()
    expect(screen.getByText('No domains configured.')).toBeInTheDocument()
  })

  it('lists saved domains with their DNS check and per-domain controls', async () => {
    renderEditor(['app.example.com'])
    expect(
      await screen.findByDisplayValue('app.example.com'),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByTestId('dns-check')).toHaveTextContent(
        'app.example.com',
      )
    })
    expect(screen.getByText('proxy tls')).toBeInTheDocument()
    expect(screen.getByTestId('basic-auth')).toBeInTheDocument()
  })

  it('adds an empty row and removes a row', async () => {
    renderEditor(['app.example.com'])
    await userEvent.click(screen.getByRole('button', { name: 'Add domain' }))
    expect(screen.getAllByLabelText('Domain')).toHaveLength(2)
    const [firstRemove] = screen.getAllByRole('button', {
      name: 'Remove domain',
    })
    if (!firstRemove) throw new Error('no remove button')
    await userEvent.click(firstRemove)
    expect(screen.getAllByLabelText('Domain')).toHaveLength(1)
  })

  it('rejects an invalid domain without saving', async () => {
    renderEditor()
    await userEvent.click(screen.getByRole('button', { name: 'Add domain' }))
    await userEvent.type(screen.getByLabelText('Domain'), 'nope')
    await userEvent.click(screen.getByRole('button', { name: 'Save domains' }))
    expect(
      await screen.findByText('Enter a valid domain, e.g. app.example.com'),
    ).toBeInTheDocument()
    expect(updateMutate).not.toHaveBeenCalled()
  })

  it('saves the trimmed domain list with the rest of the app', async () => {
    renderEditor()
    await userEvent.click(screen.getByRole('button', { name: 'Add domain' }))
    await userEvent.type(screen.getByLabelText('Domain'), 'app.example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Save domains' }))
    await waitFor(() => {
      expect(updateMutate).toHaveBeenCalled()
    })
    expect(updateMutate.mock.calls[0]?.[0]).toMatchObject({
      name: 'web',
      domains: ['app.example.com'],
    })
  })

  it('toggles the guided setup', async () => {
    renderEditor()
    expect(screen.queryByTestId('wizard')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Guided setup' }))
    expect(screen.getByTestId('wizard')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Close guide' }))
    expect(screen.queryByTestId('wizard')).not.toBeInTheDocument()
  })

  it('shows the server error when saving fails', () => {
    updateState.isError = true
    updateState.error = new Error('domain taken by another app')
    renderEditor()
    expect(screen.getByText('domain taken by another app')).toBeInTheDocument()
  })

  it('disables save and shows progress while saving', () => {
    updateState.isPending = true
    renderEditor()
    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
  })
})
