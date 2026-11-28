import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { Route as StatusRoute } from './status-page'
import { Route as ChannelsRoute } from './notification-channels'
import { Route as RegistryRoute } from './registry-credentials'
import statusEn from '../../locales/en/statusPage.json'
import settingsEn from '../../locales/en/settings.json'
import { registryCredentialListQueryOptions } from '../../queries/registryCredentials'
import { notificationChannelListQueryOptions } from '../../queries/notificationChannels'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children?: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({ children }: { children?: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))
vi.mock('../../components/StatusPageSettingsPanel', () => ({
  StatusPageSettingsPanel: () => <p>enable-card</p>,
}))
vi.mock('../../components/StatusComponentsPanel', () => ({
  StatusComponentsPanel: () => <p>components-panel</p>,
}))
vi.mock('../../hooks/useBrand', () => ({ useBrand: () => ({ DocsURL: '' }) }))
vi.mock('../../hooks/useTokenExpiryPolicy', () => ({
  useTokenExpiryPolicy: () => ({ soonDays: 7, windows: '', grace: '' }),
}))
vi.mock('../../components/TokenTable', () => ({ TokenTable: () => null }))
vi.mock('../../components/CreateTokenDialog', () => ({
  CreateTokenDialog: () => null,
}))
vi.mock('../../components/RevokedTokensCard', () => ({
  RevokedTokensCard: () => <p>revoked-card</p>,
}))
vi.mock('../../components/tokens/TokensNotices', () => ({
  TokensNotices: () => null,
}))
vi.mock('../../components/NotificationChannelTable', () => ({
  NotificationChannelTable: () => null,
}))
vi.mock('../../components/CreateNotificationChannelDialog', () => ({
  CreateNotificationChannelDialog: () => null,
}))
vi.mock('@/components/getStarted/SetupStepHint', () => ({
  SetupStepHint: () => null,
}))
vi.mock('../../components/settings/HeartbeatCard', () => ({
  HeartbeatCard: () => <p>heartbeat-card</p>,
}))
vi.mock('../../components/RegistryCredentialTable', () => ({
  RegistryCredentialTable: () => null,
}))
vi.mock('../../components/CreateRegistryCredentialDialog', () => ({
  CreateRegistryCredentialDialog: () => null,
}))
vi.mock('../../components/registry/RegistryVendorGrid', () => ({
  RegistryVendorGrid: () => null,
}))
vi.mock('../../components/registry/CloudRegistryCards', () => ({
  default: () => null,
}))
vi.mock('../../components/registry/BuiltInRegistryCard', () => ({
  BuiltInRegistryCard: () => <p>builtin-card</p>,
}))
vi.mock('../../components/registry/PackageRegistriesCard', () => ({
  PackageRegistriesCard: () => <p>packages-card</p>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['statusPage', 'settings'],
  defaultNS: 'settings',
  resources: { en: { statusPage: statusEn, settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function renderRoute(
  route: { options: { component?: unknown } },
  seed?: (qc: ReturnType<typeof createTestQueryClient>) => void,
) {
  const qc = createTestQueryClient()
  seed?.(qc)
  const Page = route.options.component as () => React.ReactNode
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <Page />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

function mockStatus(enabled: boolean, components: unknown[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      json(url.includes('/components') ? components : { enabled }),
    ),
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('status page settings', () => {
  it('shows only the enable card when off', async () => {
    mockStatus(false, [])
    renderRoute(StatusRoute)
    expect(await screen.findByText('enable-card')).toBeTruthy()
    expect(screen.queryByRole('tablist')).toBeNull()
  })
  it('shows two tabs when on without components', async () => {
    mockStatus(true, [])
    renderRoute(StatusRoute)
    expect(await screen.findAllByRole('tab')).toHaveLength(2)
  })
  it('shows four tabs when on with components', async () => {
    mockStatus(true, [{ id: 'c1' }])
    renderRoute(StatusRoute)
    expect(await screen.findAllByRole('tab')).toHaveLength(4)
  })
})

describe('notification channels page', () => {
  it('hides the heartbeat card with no channels', () => {
    renderRoute(ChannelsRoute, (qc) => {
      qc.setQueryData(notificationChannelListQueryOptions().queryKey, [])
    })
    expect(screen.queryByText('heartbeat-card')).toBeNull()
  })
  it('shows the heartbeat card with a channel', () => {
    renderRoute(ChannelsRoute, (qc) => {
      qc.setQueryData(notificationChannelListQueryOptions().queryKey, [
        { id: 'n', enabled: true },
      ] as never)
    })
    expect(screen.getByText('heartbeat-card')).toBeTruthy()
  })
})

describe('registry credentials page', () => {
  function mockRegistry(enabled: boolean) {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        json(url.includes('package-registries') ? [] : { enabled }),
      ),
    )
  }
  it('hides the extra cards with no credentials and no built-in registry', async () => {
    mockRegistry(false)
    renderRoute(RegistryRoute, (qc) => {
      qc.setQueryData(registryCredentialListQueryOptions().queryKey, [])
    })
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByText('builtin-card')).toBeNull()
    expect(screen.queryByText('packages-card')).toBeNull()
  })
  it('shows the extra cards when the built-in registry is on', async () => {
    mockRegistry(true)
    renderRoute(RegistryRoute, (qc) => {
      qc.setQueryData(registryCredentialListQueryOptions().queryKey, [])
    })
    expect(await screen.findByText('builtin-card')).toBeTruthy()
    expect(screen.getByText('packages-card')).toBeTruthy()
  })
})
