import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DomainWizard } from './DomainWizard'
import domainsEn from '../locales/en/domains.json'
import type { AppDetail } from '../types/appDetail'

const editMutate = vi.fn()
const editState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

vi.mock('../queries/apps', () => ({
  useUpdateApp: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('../queries/domainCheck', () => ({
  useDomainCheck: () => ({ data: undefined }),
}))

vi.mock('../queries/domainRedirect', () => ({
  setDomainRedirect: vi.fn(() => Promise.resolve()),
}))

vi.mock('../queries/appEnvironmentDomains', () => ({
  useAppEnvironmentDomains: () => ({ data: { environments: [] } }),
  useEditAppDomains: () => ({
    mutate: editMutate,
    ...editState,
  }),
}))

vi.mock('../queries/domainWizard', () => ({
  useIngressConnectivity: () => ({ data: undefined }),
  useListeningPorts: () => ({
    data: undefined,
    isFetching: false,
    refetch: vi.fn(),
  }),
}))

vi.mock('./DomainDnsCheck', () => ({
  DomainDnsCheck: ({ domain }: { domain: string }) => (
    <div data-testid="dns-check">{domain}</div>
  ),
}))

vi.mock('./DomainWizardSteps', () => ({
  WizardStep: ({ title, children }: { title: string; children: ReactNode }) => (
    <section>
      <h4>{title}</h4>
      {children}
    </section>
  ),
  PortStepBody: () => <div data-testid="port-step" />,
  CertificateStepBody: ({ domain }: { domain: string }) => (
    <div data-testid="cert-step">{domain}</div>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const app = { name: 'web', port: 3000 } as unknown as AppDetail

function renderWizard() {
  render(
    <I18nextProvider i18n={testI18n}>
      <DomainWizard app={app} />
    </I18nextProvider>,
  )
}

beforeEach(() => {
  editMutate.mockReset()
  editState.isPending = false
  editState.isError = false
  editState.error = null
})

describe('DomainWizard', () => {
  it('starts with nothing to check and the submit button disabled', () => {
    renderWizard()
    expect(
      screen.getByText('Enter a domain above to see the exact record to add.'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add domain' })).toBeDisabled()
    expect(screen.queryByTestId('dns-check')).not.toBeInTheDocument()
  })

  it('flags a malformed domain and keeps submit disabled', async () => {
    renderWizard()
    await userEvent.type(screen.getByLabelText('Domain'), 'not a domain')
    expect(
      screen.getByText(/Enter a valid domain, for example/),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add domain' })).toBeDisabled()
  })

  it('runs the DNS check for a valid domain once typing settles', async () => {
    renderWizard()
    await userEvent.type(screen.getByLabelText('Domain'), 'App.Example.com')
    expect(await screen.findByTestId('dns-check')).toHaveTextContent(
      'app.example.com',
    )
    expect(screen.getByRole('button', { name: 'Add domain' })).toBeEnabled()
  })

  it('adds the lowercase domain on submit', async () => {
    renderWizard()
    await userEvent.type(screen.getByLabelText('Domain'), 'app.example.com')
    const submit = screen.getByRole('button', { name: 'Add domain' })
    await waitFor(() => {
      expect(submit).toBeEnabled()
    })
    await userEvent.click(submit)
    expect(editMutate).toHaveBeenCalledWith(
      { add: ['app.example.com'], environment: undefined },
      expect.any(Object),
    )
  })

  it('offers the www counterpart for an apex and adds it when ticked', async () => {
    renderWizard()
    await userEvent.type(screen.getByLabelText('Domain'), 'example.com')
    const preset = await screen.findByRole('checkbox')
    await userEvent.click(preset)
    const submit = screen.getByRole('button', { name: 'Add domain' })
    await waitFor(() => {
      expect(submit).toBeEnabled()
    })
    await userEvent.click(submit)
    expect(editMutate).toHaveBeenCalledWith(
      { add: ['example.com', 'www.example.com'], environment: undefined },
      expect.any(Object),
    )
  })

  it('shows the server error when adding fails', () => {
    editState.isError = true
    editState.error = new Error('domain already in use')
    renderWizard()
    expect(screen.getByText('domain already in use')).toBeInTheDocument()
  })

  it('shows the pending label while saving', () => {
    editState.isPending = true
    renderWizard()
    expect(screen.getByRole('button', { name: 'Adding...' })).toBeDisabled()
  })
})
