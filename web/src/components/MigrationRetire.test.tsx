import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MigrationRetire from './MigrationRetire'
import migrationEn from '../locales/en/migration.json'
import type { AppImportSession } from '../queries/appImport'
import type { RetireReport } from '../queries/migrationJob'
import { createTestQueryClient } from '@/test/testQueryClient'

function bodyOf(init: RequestInit | undefined): unknown {
  return typeof init?.body === 'string' ? JSON.parse(init.body) : null
}

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

const session = { id: 'appimp-1' } as AppImportSession

const red: RetireReport = {
  safe_to_delete: false,
  fallback: [
    'Keep the old instance running and untouched until every gate here is green.',
  ],
  gates: [
    { id: 'staged', status: 'pass', detail: '1 app(s) built and healthy here' },
    {
      id: 'volumes',
      status: 'fail',
      detail: '1 of 1 volume(s) are not copied',
      open: ['web: uploads'],
      fix: 'copy the volumes again right before the switch',
    },
    {
      id: 'dns',
      status: 'pending',
      detail: 'the live check has not run yet',
      fix: 'run the check now',
    },
    {
      id: 'source-backup',
      status: 'manual',
      detail: 'Keep a backup',
      fix: 'Copy it, then confirm.',
    },
  ],
}

const green: RetireReport = {
  safe_to_delete: true,
  fallback: red.fallback,
  gates: red.gates.map((g) => ({
    ...g,
    status: 'pass',
    open: undefined,
    fix: undefined,
  })),
}

function stubFetch(handler: (url: string, init?: RequestInit) => RetireReport) {
  const fn = vi.fn((url: string, init?: RequestInit) =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(handler(url, init)),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderStep() {
  const queryClient = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <MigrationRetire session={session} onBack={vi.fn()} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('MigrationRetire', () => {
  beforeEach(() => window.localStorage.clear())
  afterEach(() => vi.unstubAllGlobals())

  it('stays red and lists what is open, with the rollback guidance', async () => {
    stubFetch(() => red)
    renderStep()
    expect(await screen.findByText('Not safe to delete yet')).toBeTruthy()
    expect(screen.getByText('web: uploads')).toBeTruthy()
    expect(screen.getByText(/copy the volumes again/)).toBeTruthy()
    expect(screen.getByText(/Keep the old instance running/)).toBeTruthy()
    expect(screen.queryByText('Every gate passed')).toBeNull()
  })

  it('runs the live checks only when asked and shows the verdict it returns', async () => {
    const user = userEvent.setup()
    const fn = stubFetch((_, init) => (init?.method === 'POST' ? green : red))
    renderStep()
    await screen.findByText('Not safe to delete yet')
    expect(fn.mock.calls.every(([, init]) => init?.method !== 'POST')).toBe(
      true,
    )

    await user.click(
      screen.getByRole('button', { name: 'Run the live checks' }),
    )
    expect(await screen.findByText('Every gate passed')).toBeTruthy()
    const post = fn.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(bodyOf(post?.[1])).toMatchObject({ probe: true })
  })

  it('sends a confirmation and keeps it for the next visit', async () => {
    const user = userEvent.setup()
    const fn = stubFetch((_, init) => (init?.method === 'POST' ? green : red))
    renderStep()
    await user.click(await screen.findByRole('button', { name: 'I did this' }))
    await waitFor(() =>
      expect(fn.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(
        true,
      ),
    )
    const post = fn.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(bodyOf(post?.[1])).toMatchObject({
      probe: false,
      acknowledged: ['source-backup'],
    })
    expect(window.localStorage.getItem('migration-acks-appimp-1')).toContain(
      'source-backup',
    )
  })

  it('only offers a confirmation for steps a person has to do', async () => {
    stubFetch(() => red)
    renderStep()
    await screen.findByText('Not safe to delete yet')
    expect(screen.getAllByRole('button', { name: 'I did this' })).toHaveLength(
      1,
    )
  })
})
