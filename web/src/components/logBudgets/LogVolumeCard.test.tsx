import { Suspense } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { LogVolumeCard } from './LogVolumeCard'
import logBudgetsEn from '../../locales/en/logBudgets.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => false }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['logBudgets'],
  defaultNS: 'logBudgets',
  resources: { en: { logBudgets: logBudgetsEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => vi.restoreAllMocks())

describe('LogVolumeCard', () => {
  it('lists apps with usage, forecast and trimmed lines', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          default_budget_bytes: 0,
          apps: [
            {
              app: 'noisy',
              bytes: 2097152,
              lines: 10,
              rate_bytes_per_min: 1024,
              budget_bytes: 4194304,
              budget_custom: true,
              percent: 50,
              forecast_days: 1.5,
              trimmed_lines: 7,
              trimmed_bytes: 100,
            },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    const qc = createTestQueryClient()
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={qc}>
          <Suspense fallback={null}>
            <LogVolumeCard />
          </Suspense>
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(await screen.findByText('noisy')).toBeTruthy()
    expect(screen.getByText('2.0 MiB')).toBeTruthy()
    expect(screen.getByText('1.5 days')).toBeTruthy()
    expect(screen.getByText('7 lines')).toBeTruthy()
  })
})
