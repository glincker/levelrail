import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/plugins.json'
import type {
  MarketplaceEntry,
  MarketplacePreview,
} from '../../queries/pluginMarketplace'
import { MarketplaceInstallDialog } from './MarketplaceInstallDialog'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['plugins'],
  defaultNS: 'plugins',
  resources: { en: { plugins: en } },
  interpolation: { escapeValue: false },
})

const digest = 'sha256:' + '1'.repeat(64)

const entry: MarketplaceEntry = {
  id: 'example-notify',
  name: 'Example notify',
  publisher: 'Example Org',
  publisher_key_id: 'ed25519:abc',
  publisher_key_name: 'Example Org',
  version: '1.0.0',
  summary: 'Delivers alerts',
  categories: [],
  icon: '',
  abilities: ['read'],
  outbound_hosts: [],
  digest,
  changelog: '',
  min_version: '',
  source_id: 'default',
  source_name: 'Default',
  verified: true,
  key_trusted: false,
  installed: false,
  installed_version: '',
  update_available: false,
  compatible: true,
}

const preview: MarketplacePreview = {
  entry,
  source_id: 'default',
  source_name: 'Default',
  key_id: 'ed25519:abc',
  key_name: 'Example Org',
  key_trusted: false,
  action: 'install',
  installed_version: '',
  added_abilities: ['read'],
  added_hosts: [],
  reapproval_required: false,
}

function renderSheet(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock)
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MarketplaceInstallDialog entry={entry} onClose={vi.fn()} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('MarketplaceInstallDialog', () => {
  it('needs the publisher key to be trusted before install, then sends only the shown abilities', async () => {
    const fetchMock = vi.fn<
      (url: string, init?: RequestInit) => Promise<Response>
    >((url) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            url.endsWith('/install')
              ? { plugin: {}, action: 'install', trust_added: true }
              : preview,
          ),
          { status: url.endsWith('/install') ? 201 : 200 },
        ),
      ),
    )
    renderSheet(fetchMock)
    const install = await screen.findByRole('button', {
      name: 'Install and enable',
    })
    expect((install as HTMLButtonElement).disabled).toBe(true)
    await userEvent.click(screen.getByRole('checkbox'))
    expect((install as HTMLButtonElement).disabled).toBe(false)
    await userEvent.click(install)
    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([u]) => u.endsWith('/install'))
      expect(call).toBeTruthy()
    })
    const call = fetchMock.mock.calls.find(([u]) => u.endsWith('/install'))
    const init = call?.[1] as unknown as RequestInit
    expect(JSON.parse(init.body as string)).toEqual({
      source_id: 'default',
      id: 'example-notify',
      version: '1.0.0',
      digest,
      abilities: ['read'],
      trust_key_id: 'ed25519:abc',
    })
  })

  it('shows an error with Retry when the listing cannot be verified', async () => {
    renderSheet(
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: 'bad signature' }), {
          status: 422,
        }),
      ),
    )
    expect(
      await screen.findByText('Could not verify this listing'),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
})
