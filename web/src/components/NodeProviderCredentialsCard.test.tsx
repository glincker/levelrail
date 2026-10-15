import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../locales/en/settings.json'
import { NodeProviderCredentialsCard } from './NodeProviderCredentialsCard'
import type { NodeProviderResource } from '../types/nodeProvision'

const providers: NodeProviderResource[] = [
  {
    provider: 'hetzner',
    has_token: true,
    status: 'connected',
    methods: ['api_token'],
    method: 'api_token',
    connected_by: 'Ada',
  },
  {
    provider: 'digitalocean',
    has_token: false,
    methods: ['api_token'],
  },
  {
    provider: 'aws',
    has_token: false,
    methods: ['access_key', 'assume_role', 'ambient'],
  },
  {
    provider: 'azure',
    has_token: false,
    methods: ['service_account_json', 'workload_identity_federation'],
  },
  { provider: 'custom', has_token: false, methods: ['manual'] },
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

const router = vi.hoisted(() => {
  const state = {
    search: {} as Record<string, unknown>,
    listeners: new Set<() => void>(),
  }
  return state
})

vi.mock('@tanstack/react-router', async () => {
  const React = await import('react')
  return {
    useSearch: () =>
      React.useSyncExternalStore(
        (cb) => {
          router.listeners.add(cb)
          return () => router.listeners.delete(cb)
        },
        () => router.search,
      ),
    useNavigate: () => (opts: { search: Record<string, unknown> }) => {
      router.search = opts.search
      router.listeners.forEach((l) => l())
      return Promise.resolve()
    },
  }
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
const idle = { mutate: vi.fn(), isPending: false, isError: false }
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
  useTestNodeProvider: () => idle,
  useDisconnectNodeProvider: () => idle,
}))

describe('NodeProviderCredentialsCard', () => {
  beforeEach(() => {
    router.search = {}
  })
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
    expect(screen.getAllByText('Not connected')).toHaveLength(4)
    expect(
      screen.getByRole('button', { name: 'Configure Hetzner' }),
    ).toHaveTextContent('Update')
    expect(screen.getByText('Connected by Ada')).toBeVisible()
  })

  it('opens from the URL and writes the param when a row is clicked', () => {
    renderCard()
    open('DigitalOcean')
    expect(router.search).toMatchObject({ connect: 'digitalocean' })
    expect(screen.getByLabelText('API token')).toBeVisible()
  })

  it('opens on load from a deep link and honors a method alias', () => {
    router.search = { connect: 'aws', method: 'role' }
    renderCard()
    expect(screen.getByLabelText('Role ARN')).toBeVisible()
  })

  it('removes the dialog params when closed', () => {
    router.search = { connect: 'digitalocean', other: 'keep' }
    renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(router.search).toEqual({ other: 'keep' })
  })

  it('offers only the methods the provider supports', () => {
    router.search = { connect: 'azure' }
    renderCard()
    expect(screen.getAllByRole('radio')).toHaveLength(2)
    expect(screen.getByLabelText('Credential (JSON)')).toBeVisible()
  })

  it('submits a verified token connection', () => {
    router.search = { connect: 'digitalocean' }
    renderCard()
    fireEvent.change(screen.getByLabelText('API token'), {
      target: { value: 'do-secret-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Test and connect' }))
    expect(setCredentialMutate).toHaveBeenCalledWith(
      {
        provider: 'digitalocean',
        verify: true,
        method: 'api_token',
        token: 'do-secret-token',
      },
      expect.anything(),
    )
  })

  it('submits an access key and secret for aws', () => {
    router.search = { connect: 'aws' }
    renderCard()
    fireEvent.change(screen.getByLabelText('Access key ID'), {
      target: { value: 'AKIA...' },
    })
    fireEvent.change(screen.getByLabelText('Secret access key'), {
      target: { value: 'shh' },
    })
    fireEvent.change(screen.getByLabelText('Region (optional)'), {
      target: { value: 'eu-west-1' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Test and connect' }))
    expect(setCredentialMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: 'aws',
        method: 'access_key',
        token: 'AKIA...',
        secret_access_key: 'shh',
        region: 'eu-west-1',
      }),
      expect.anything(),
    )
  })

  it('skips the key fields for the ambient identity method', () => {
    router.search = { connect: 'aws', method: 'ambient' }
    renderCard()
    expect(screen.queryByLabelText('Access key ID')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Test and connect' }))
    expect(setCredentialMutate).toHaveBeenCalledWith(
      expect.objectContaining({ provider: 'aws', method: 'ambient' }),
      expect.anything(),
    )
  })

  it('records a custom data center without any credential', () => {
    router.search = { connect: 'custom' }
    renderCard()
    fireEvent.change(screen.getByLabelText('Data center name'), {
      target: { value: 'Rack 4' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save data center' }))
    expect(setCredentialMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: 'custom',
        method: 'manual',
        label: 'Rack 4',
        verify: false,
      }),
      expect.anything(),
    )
  })
})
