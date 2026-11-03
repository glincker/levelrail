import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../../locales/en/updates.json'
import { MIGRATION_PREVIEW_COUNT, UpdateMigrations } from './UpdateMigrations'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const mig = (n: number) => ({ version: n, name: `add_table_${String(n)}` })

function mockFetch(
  plan: Record<string, unknown>,
  dry: { status: number; body: unknown },
) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const json = (status: number, body: unknown) =>
        Promise.resolve(
          new Response(JSON.stringify(body), {
            status,
            headers: { 'Content-Type': 'application/json' },
          }),
        )
      if (url.includes('migration-dry-run')) return json(dry.status, dry.body)
      if (url.includes('self-upgrade/plan')) return json(200, plan)
      return json(200, { running: null, current_version: 'v1' })
    }),
  )
}

function view() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <UpdateMigrations />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('UpdateMigrations', () => {
  it('renders the list with a Show all disclosure', async () => {
    const list = Array.from({ length: MIGRATION_PREVIEW_COUNT + 2 }, (_, i) =>
      mig(i + 1),
    )
    mockFetch(
      { pending_migrations: list.length, pending_migration_list: list },
      { status: 200, body: {} },
    )
    view()
    expect(await screen.findByText(/Migration 1: add_table_1/)).toBeTruthy()
    expect(screen.queryByText(/Migration 7:/)).toBeNull()
    await userEvent.click(screen.getByText(/Show all 7/))
    expect(screen.getByText(/Migration 7:/)).toBeTruthy()
  })

  it('falls back to the count without the list field', async () => {
    mockFetch({ pending_migrations: 3 }, { status: 200, body: {} })
    view()
    expect(
      await screen.findByText('3 database migrations will run'),
    ).toBeTruthy()
  })

  it('shows none when zero', async () => {
    mockFetch({ pending_migrations: 0 }, { status: 200, body: {} })
    view()
    expect(await screen.findByText('No database migrations')).toBeTruthy()
  })

  it('reports a clean dry run', async () => {
    mockFetch(
      { pending_migrations: 1, pending_migration_list: [mig(1)] },
      {
        status: 200,
        body: { ok: true, applied: [mig(1)], duration_ms: 1500 },
      },
    )
    view()
    await userEvent.click(await screen.findByText('Test migrations on a copy'))
    expect(await screen.findByText(/applied cleanly to a copy/)).toBeTruthy()
    expect(screen.getByText(/Took 1.5 s/)).toBeTruthy()
  })

  it('reports the failing migration', async () => {
    mockFetch(
      {
        pending_migrations: 2,
        pending_migration_list: [mig(1), mig(2)],
      },
      {
        status: 200,
        body: {
          ok: false,
          applied: [mig(1)],
          duration_ms: 10,
          error: 'no such column',
        },
      },
    )
    view()
    await userEvent.click(await screen.findByText('Test migrations on a copy'))
    expect(
      await screen.findByText(
        /Migration 2 \(add_table_2\) failed: no such column/,
      ),
    ).toBeTruthy()
    expect(screen.getByText('Test again')).toBeTruthy()
  })

  it('hides the button when the server lacks the endpoint', async () => {
    mockFetch(
      { pending_migrations: 1, pending_migration_list: [mig(1)] },
      { status: 404, body: { error: 'not found' } },
    )
    view()
    await userEvent.click(await screen.findByText('Test migrations on a copy'))
    await waitFor(() => {
      expect(screen.queryByText('Test again')).toBeNull()
    })
    expect(screen.queryByText('Test migrations on a copy')).toBeNull()
  })
})
