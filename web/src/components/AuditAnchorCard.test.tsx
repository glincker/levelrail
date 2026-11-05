import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AuditAnchorCard } from './AuditAnchorCard'
import auditEn from '../locales/en/auditLog.json'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['auditLog'],
  defaultNS: 'auditLog',
  resources: { en: { auditLog: auditEn } },
  interpolation: { escapeValue: false },
})

function mockStatus(status: number, body: object) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify(body), { status })),
    ),
  )
}

function renderCard() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <AuditAnchorCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const base = {
  enabled: true,
  last_exported_seq: 42,
  last_exported_at: '2026-11-01T10:00:00Z',
  exported: 40,
  failed: 2,
  dropped: 1,
  queued: 3,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AuditAnchorCard', () => {
  it('shows Off with a link to the setting', async () => {
    mockStatus(200, { ...base, enabled: false })
    renderCard()
    expect(await screen.findByText('Off')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Set the anchor webhook' }),
    ).toBeInTheDocument()
  })

  it('shows counts, sequence and the last error when exporting', async () => {
    mockStatus(200, { ...base, last_error: 'webhook returned 500' })
    renderCard()
    expect(await screen.findByText('Exporting')).toBeInTheDocument()
    expect(screen.getByText(/Last exported sequence 42/)).toBeInTheDocument()
    expect(screen.getByText('Queued')).toBeInTheDocument()
    expect(screen.getByText(/webhook returned 500/)).toBeInTheDocument()
  })

  it('omits the error callout when there is none', async () => {
    mockStatus(200, base)
    renderCard()
    await screen.findByText('Exporting')
    expect(screen.queryByText('Last error')).not.toBeInTheDocument()
  })

  it.each([403, 404])('hides the card on %i', async (code) => {
    mockStatus(code, { error: 'nope' })
    const { container } = renderCard()
    await new Promise((r) => setTimeout(r, 50))
    expect(container.querySelector('section')).toBeNull()
  })
})
