import { cleanup, render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../locales/en/settings.json'
import commonEn from '../locales/en/common.json'
import { DockerGuardCard } from './DockerGuardCard'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'common'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function renderCard(extra: Record<string, unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(
      () =>
        new Response(
          JSON.stringify({
            mode: 'audit',
            source: 'default',
            effective: 'audit',
            running: true,
            restart_required: false,
            rules: [],
            configured: true,
            window_seconds: 604800,
            window: [],
            would_deny_total: 0,
            denied_total: 0,
            ready_to_enforce: false,
            ...extra,
          }),
          { status: 200 },
        ),
    ),
  )
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <DockerGuardCard />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('DockerGuardCard window', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('says unknown, not nothing flagged, when the audit log could not be read', async () => {
    renderCard({ window_error: 'audit log read failed' })
    expect(
      await screen.findByText(/Unknown: the audit log/),
    ).toBeInTheDocument()
    expect(screen.queryByText('Nothing flagged.')).not.toBeInTheDocument()
  })

  it('says nothing flagged for a readable empty window', async () => {
    renderCard({})
    expect(await screen.findByText('Nothing flagged.')).toBeInTheDocument()
  })
})
