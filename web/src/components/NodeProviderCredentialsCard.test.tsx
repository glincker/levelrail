import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../locales/en/settings.json'
import { NodeProviderCredentialsCard } from './NodeProviderCredentialsCard'
import type { NodeProviderResource } from '../types/nodeProvision'

const providers: NodeProviderResource[] = [
  { provider: 'hetzner', has_token: true },
  { provider: 'digitalocean', has_token: false },
  { provider: 'aws', has_token: false },
  { provider: 'azure', has_token: false },
]

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function renderCard() {
  return render(
    <I18nextProvider i18n={testI18n}>
      <NodeProviderCredentialsCard />
    </I18nextProvider>,
  )
}

vi.mock('@/components/HelpLink', () => ({
  HelpLink: ({ label }: { label: string }) => <a href="#help">{label}</a>,
}))

const setCredentialMutate = vi.fn()
vi.mock('../queries/nodeProvision', () => ({
  useNodeProviders: () => ({
    data: providers,
    isLoading: false,
    isError: false,
  }),
  useSetNodeProviderCredential: () => ({
    mutate: setCredentialMutate,
    isPending: false,
    isError: false,
  }),
}))

describe('NodeProviderCredentialsCard', () => {
  afterEach(() => {
    cleanup()
    setCredentialMutate.mockReset()
  })

  function open(name: string) {
    fireEvent.click(screen.getByRole('button', { name: `Configure ${name}` }))
  }

  it('lists every provider with its connection state', () => {
    renderCard()
    expect(screen.getByText('Connected')).toBeVisible()
    expect(screen.getAllByText('Not connected')).toHaveLength(3)
    expect(
      screen.getByRole('button', { name: 'Configure Hetzner' }),
    ).toHaveTextContent('Update')
  })

  it('renders a single token field for a plain API token provider', () => {
    renderCard()
    open('DigitalOcean')
    expect(screen.getAllByLabelText('API token')).toHaveLength(1)
  })

  it('shows only the JSON credential for azure, not a second API token', () => {
    renderCard()
    open('Azure')
    expect(screen.getByLabelText('Service principal (JSON)')).toBeVisible()
    expect(screen.queryByLabelText('API token')).not.toBeInTheDocument()
  })

  it('submits a new token for the chosen provider', () => {
    renderCard()
    open('DigitalOcean')
    fireEvent.change(screen.getByLabelText('API token'), {
      target: { value: 'do-secret-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(setCredentialMutate).toHaveBeenCalledWith(
      { provider: 'digitalocean', token: 'do-secret-token' },
      expect.anything(),
    )
  })

  it('submits an access key and secret for aws', () => {
    renderCard()
    open('AWS')
    fireEvent.change(screen.getByLabelText('Access key ID'), {
      target: { value: 'AKIA...' },
    })
    fireEvent.change(screen.getByLabelText('Secret access key'), {
      target: { value: 'shh' },
    })
    fireEvent.change(screen.getByLabelText('Region (optional)'), {
      target: { value: 'eu-west-1' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(setCredentialMutate).toHaveBeenCalledWith(
      {
        provider: 'aws',
        token: 'AKIA...',
        secret_access_key: 'shh',
        session_token: undefined,
        region: 'eu-west-1',
        role_arn: undefined,
        use_ambient_credentials: false,
      },
      expect.anything(),
    )
  })

  it('skips the access key fields for aws when using ambient credentials', () => {
    renderCard()
    open('AWS')
    fireEvent.click(
      screen.getByRole('checkbox', {
        name: /Use this control plane's own AWS identity/,
      }),
    )
    expect(screen.queryByLabelText('Access key ID')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Secret access key')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(setCredentialMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: 'aws',
        use_ambient_credentials: true,
      }),
      expect.anything(),
    )
  })
})
