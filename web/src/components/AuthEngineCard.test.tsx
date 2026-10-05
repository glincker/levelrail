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
  mode: 'shadow',
  library_version: 'v2.6.0',
  areas: [],
  compared: 12,
  matched: 11,
  mismatched: 1,
  dropped: 0,
  skipped: 0,
  errors: 0,
  mismatches: [
    {
      at: '2026-10-05T10:00:00Z',
      kind: 'abilities',
      token_id: 'tok_abc',
      legacy_accepted: true,
      library_accepted: true,
      legacy_abilities: ['read', 'write'],
      library_abilities: ['read'],
    },
  ],
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
  it('shows the mode, counters and a mismatch in shadow mode', async () => {
    stub(200, base)
    renderCard()
    expect(await screen.findByText('Shadow')).toBeInTheDocument()
    expect(screen.getByText('Mismatched')).toBeInTheDocument()
    expect(screen.getByText('tok_abc')).toBeInTheDocument()
    expect(screen.getByText('Abilities')).toBeInTheDocument()
  })

  it('hides the counters outside shadow mode', async () => {
    stub(200, { ...base, mode: 'legacy', mismatches: [] })
    renderCard()
    expect(await screen.findByText('Built-in')).toBeInTheDocument()
    expect(screen.queryByText('Compared')).not.toBeInTheDocument()
  })

  it('renders nothing for a non-root user', async () => {
    stub(403, { error: 'forbidden' })
    const { container } = renderCard()
    await vi.waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })
})
