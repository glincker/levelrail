import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type { ReleaseHistory as History } from '../../queries/releases'
import { ReleaseHistory } from './ReleaseHistory'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const item = {
  url: '',
  published_at: '2026-10-01T00:00:00Z',
  channel: 'stable' as const,
  running: false,
  retained: false,
  asset_name: 'bin',
  asset_available: true,
  asset_size: 1024 * 1024 * 40,
  signed: true,
  schema_version: 100,
  schema_source: 'manifest' as const,
  notes: '',
}

function history(overrides: Partial<History>): History {
  return {
    current_version: 'v1.5.0',
    current_schema_version: 100,
    channel: 'stable',
    view: 'stable',
    github_reachable: true,
    releases: [],
    retained_only: [],
    ...overrides,
  }
}

function renderWith(data: History) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(data),
      }),
    ),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <ReleaseHistory />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ReleaseHistory', () => {
  it('shows each release with its schema verdict and disables the running one', async () => {
    renderWith(
      history({
        releases: [
          { ...item, version: 'v1.5.0', running: true, verdict: 'binary_only' },
          {
            ...item,
            version: 'v1.0.0',
            schema_version: 90,
            verdict: 'restore_required',
          },
          {
            ...item,
            version: 'v0.9.0',
            schema_version: null,
            verdict: 'unknown',
            schema_source: 'unknown',
          },
        ],
      }),
    )
    expect(await screen.findByText('v1.0.0')).toBeInTheDocument()
    expect(screen.getByText('Needs a backup restore')).toBeInTheDocument()
    expect(screen.getByText('Compatibility unknown')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'This is the running release' }),
    ).toBeDisabled()
  })

  it('explains an empty channel and an unreachable GitHub', async () => {
    renderWith(history({ github_reachable: false }))
    expect(
      await screen.findByText('No releases found for this channel.'),
    ).toBeInTheDocument()
    expect(screen.getByText(/GitHub could not be reached/)).toBeInTheDocument()
  })
})
