import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AuthEngineCard } from './AuthEngineCard'
import settingsEn from '../locales/en/settings.json'
import type { AuthEngineStatus } from '../queries/authEngine'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const base: AuthEngineStatus = {
  library_version: 'v2.7.0',
  totp: true,
  passkeys: false,
  oauth: true,
}

function stub(status: number, body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: status >= 200 && status < 300,
        status,
        json: () => Promise.resolve(body),
      } as unknown as Response),
    ),
  )
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <AuthEngineCard />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AuthEngineCard', () => {
  it('shows which sign-in features are available', async () => {
    stub(200, base)
    renderCard()
    expect(await screen.findByText('Two-factor (TOTP)')).toBeInTheDocument()
    expect(screen.getAllByText('Available')).toHaveLength(2)
    expect(screen.getAllByText('Unavailable')).toHaveLength(1)
  })

  it('renders nothing for a non-root user', async () => {
    stub(403, { error: 'forbidden' })
    const { container } = renderCard()
    await vi.waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })
})
