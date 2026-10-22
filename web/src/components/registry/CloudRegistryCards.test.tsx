import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../../locales/en/settings.json'
import { CloudRegistryCards } from './CloudRegistryCards'
import type { RegistryCredential } from '../../types/registryCredential'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function renderCards(credentials: RegistryCredential[]) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <CloudRegistryCards credentials={credentials} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('CloudRegistryCards', () => {
  it('tests a preset with its default region and shows the expiry', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          registry_host: '1.dkr.ecr.us-east-1.amazonaws.com',
          username: 'AWS',
          expires_at: '2030-01-01T00:00:00Z',
        }),
        { status: 200 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderCards([])

    expect(screen.getByText('Connect ECR')).toBeInTheDocument()
    expect(screen.getByText('Connect Artifact Registry')).toBeInTheDocument()
    expect(screen.getByText('Connect ACR')).toBeInTheDocument()

    fireEvent.click(screen.getAllByRole('button', { name: 'Test' })[0]!)
    expect(
      await screen.findByText(/1\.dkr\.ecr\.us-east-1\.amazonaws\.com/),
    ).toBeInTheDocument()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/registry-credentials/presets/test')
    expect(JSON.parse(init.body as string)).toEqual({
      preset: 'ecr',
      region: 'us-east-1',
    })
  })

  it('needs a host for ACR before it can test or connect', () => {
    renderCards([])
    const buttons = screen.getAllByRole('button', { name: 'Test' })
    expect(buttons[2]).toBeDisabled()
  })

  it('shows a failing refresh with a Refresh now fix button', () => {
    renderCards([
      {
        id: 'regcred_1',
        name: 'prod-ecr',
        registry_host: 'x.dkr.ecr.us-east-1.amazonaws.com',
        username: 'AWS',
        created_at: '2030-01-01T00:00:00Z',
        preset: 'ecr',
        last_refresh_error: 'access denied',
      },
    ])
    expect(screen.getByText('Refresh failing')).toBeInTheDocument()
    expect(screen.getByText('access denied')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Refresh now/ }),
    ).toBeInTheDocument()
  })
})
