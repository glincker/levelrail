import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { Route } from './alerts'
import inboxEn from '../locales/en/inbox.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children }: { children?: React.ReactNode }) => (
    <a href="/">{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['inbox'],
  defaultNS: 'inbox',
  resources: { en: { inbox: inboxEn } },
  interpolation: { escapeValue: false },
})

const entry = {
  id: 'h1',
  at: '2026-11-16T10:00:00Z',
  app: 'web',
  event: 'fired',
}

function mockApi(opts: { inbox: unknown[]; history: unknown[] }) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      let body: unknown = []
      if (url.startsWith('/api/v1/inbox/preferences'))
        body = {
          min_severity: 'warning',
          muted_apps: [],
          digest_enabled: false,
          digest_hour: 8,
          digest_channel_id: '',
        }
      else if (url.startsWith('/api/v1/inbox?')) body = opts.inbox
      else if (url.startsWith('/api/v1/alert-history')) body = opts.history
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

function renderPage() {
  const Page = Route.options.component as () => React.ReactNode
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>
        <Page />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('Alerts page states', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows no tabs or buttons while loading', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => undefined)),
    )
    renderPage()
    expect(screen.queryByRole('tablist')).toBeNull()
    expect(screen.queryByRole('link')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('shows one calm empty state with a link row and no tabs', async () => {
    mockApi({ inbox: [], history: [] })
    renderPage()
    expect(await screen.findByText('No alerts yet')).toBeTruthy()
    expect(screen.getByText('Set up notification channels')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Silences' })).toBeTruthy()
    expect(screen.queryByRole('tablist')).toBeNull()
  })

  it('shows tabs without a count when history exists but nothing is unread', async () => {
    mockApi({ inbox: [], history: [entry] })
    renderPage()
    expect(await screen.findByRole('tablist')).toBeTruthy()
    expect(screen.getByRole('tab', { name: 'Inbox' })).toBeTruthy()
    expect(screen.queryByText('No alerts yet')).toBeNull()
  })

  it('shows the unread count on the Inbox tab when alerts are unread', async () => {
    mockApi({
      inbox: [{ ...entry, read: false, resolved: false }],
      history: [entry],
    })
    renderPage()
    expect(
      await screen.findByRole('tab', { name: 'Inbox (1 unread)' }),
    ).toBeTruthy()
  })
})
