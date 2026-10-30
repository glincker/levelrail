import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CoolifyExitReportPanel } from './CoolifyExitReport'
import migrationEn from '../locales/en/migration.json'
import type { CoolifyExitReport } from '../queries/coolifyExit'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

const report: CoolifyExitReport = {
  mode: 'dry-run',
  ready_to_retire: false,
  counts: { imported: 1, manual: 1, skipped: 1 },
  rows: [
    { kind: 'app', name: 'web', outcome: 'imported' },
    {
      kind: 'volume',
      name: 'web/uploads',
      outcome: 'manual',
      reasons: ['the volume is created empty at /app/uploads'],
      next: ['copy the data'],
    },
    {
      kind: 'database',
      name: 'analytics',
      outcome: 'skipped',
      reasons: ['ClickHouse is not a managed database engine here'],
    },
  ],
  waves: [
    {
      number: 1,
      title: 'Stateless apps',
      why: 'nothing to lose',
      apps: [{ name: 'web', domains: ['web.example.com'] }],
    },
  ],
  checklist: [
    { id: 'stage', title: 'Stage everything', detail: 'No DNS yet.' },
  ],
}

function renderPanel() {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <CoolifyExitReportPanel sessionId="appimp-1" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('CoolifyExitReportPanel', () => {
  it('loads the report on demand and groups rows by outcome', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify(report), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    renderPanel()
    expect(fetchMock).not.toHaveBeenCalled()
    await userEvent.click(
      screen.getByRole('button', { name: 'Show exit report' }),
    )
    expect(await screen.findByText('web/uploads')).toBeTruthy()
    expect(
      screen.getByText('ClickHouse is not a managed database engine here'),
    ).toBeTruthy()
    expect(screen.getByText('Stage everything')).toBeTruthy()
    expect(screen.getByText(/Stateless apps/)).toBeTruthy()
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/v1/migration/apps/sessions/appimp-1/exit-report',
    )
  })

  it('shows the reason when the session is not connected', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: 'reconnect it first' }), {
          status: 409,
        }),
      ),
    )
    renderPanel()
    await userEvent.click(
      screen.getByRole('button', { name: 'Show exit report' }),
    )
    expect(await screen.findByText(/reconnect it first/)).toBeTruthy()
  })
})
