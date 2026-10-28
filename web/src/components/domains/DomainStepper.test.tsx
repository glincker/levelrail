import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DomainStepper } from './DomainStepper'
import domainsEn from '../../locales/en/domains.json'

const editMutate = vi.fn()
const check = vi.hoisted(() => ({
  data: undefined as Record<string, unknown> | undefined,
  capped: false,
}))
const cert = vi.hoisted(() => ({
  data: undefined as Record<string, unknown> | undefined,
}))

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => (
    <a href="/logs">{children}</a>
  ),
}))
vi.mock('@tanstack/react-query', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({ data: cert.data }),
}))
vi.mock('../../queries/appEnvironmentDomains', () => ({
  useEditAppDomains: () => ({
    mutate: editMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
}))
vi.mock('../../queries/domainStepper', () => ({
  useStepperDomainCheck: () => ({
    data: check.data,
    capped: check.capped,
    isFetching: false,
    isError: false,
    recheck: vi.fn(),
  }),
}))
vi.mock('../../queries/serverAddress', () => ({
  useServerAddress: () => ({
    data: {
      records: [{ type: 'A', name: 'app.example.com', value: '203.0.113.9' }],
    },
  }),
}))

async function setup(initialDomain = '') {
  const i18n = i18next.createInstance()
  await i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { domains: domainsEn } },
    defaultNS: 'domains',
    interpolation: { escapeValue: false },
  })
  return render(
    <I18nextProvider i18n={i18n}>
      <DomainStepper appName="shop" initialDomain={initialDomain} />
    </I18nextProvider>,
  )
}

describe('DomainStepper', () => {
  beforeEach(() => {
    editMutate.mockReset()
    check.data = undefined
    check.capped = false
    cert.data = undefined
  })

  it('validates the domain and offers the www counterpart', async () => {
    await setup()
    const add = screen.getByRole('button', { name: /add and continue/i })
    expect(add).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Domain'), 'example.com')
    expect(add).toBeEnabled()
    expect(screen.getByText(/use www.example.com/i)).toBeInTheDocument()
    await userEvent.click(add)
    expect(editMutate).toHaveBeenCalledWith(
      { add: ['example.com'] },
      expect.anything(),
    )
  })

  it('shows records with the picked provider hint', async () => {
    await setup('app.example.com')
    expect(screen.getByText('203.0.113.9')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Namecheap' }))
    expect(screen.getByText(/Advanced DNS/)).toBeInTheDocument()
  })

  it('shows expected vs resolved and the capped notice', async () => {
    check.data = {
      status: 'propagating',
      expected_ipv4: ['203.0.113.9'],
      resolved_hosts: ['198.51.100.1'],
    }
    check.capped = true
    await setup('app.example.com')
    await userEvent.click(
      screen.getByRole('button', { name: /added the records/i }),
    )
    expect(screen.getByText('198.51.100.1')).toBeInTheDocument()
    expect(screen.getByText(/automatic checking stopped/i)).toBeInTheDocument()
  })

  it('reports a failed certificate with its reason', async () => {
    check.data = { status: 'connected' }
    cert.data = {
      state: 'failed',
      error: 'rate limited',
      hint: 'Wait an hour.',
    }
    await setup('app.example.com')
    await userEvent.click(
      screen.getByRole('button', { name: /added the records/i }),
    )
    await userEvent.click(screen.getByRole('button', { name: /continue/i }))
    expect(screen.getByText('Certificate failed')).toBeInTheDocument()
    expect(screen.getByText('rate limited')).toBeInTheDocument()
    expect(screen.getByText('Wait an hour.')).toBeInTheDocument()
  })
})
