import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppHealthExplainLine } from './AppHealthExplainLine'
import commonEn from '../locales/en/common.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

function stubExplain(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      } as unknown as Response),
    ),
  )
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <AppHealthExplainLine appName="api" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AppHealthExplainLine', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('names the failing check, restarts and next step', async () => {
    stubExplain({
      app_name: 'api',
      state: 'failing',
      failing_check: {
        target: '/healthz',
        status_code: 404,
        since: '2026-11-21T10:00:00Z',
        last_probed: '2026-11-21T10:05:00Z',
      },
      restarts: 4,
      crashloop: true,
      log_error: 'boom',
      next_step: { kind: 'rollback', command: 'levelrail apps rollback api' },
      computed_at: '2026-11-21T10:05:00Z',
    })
    const line = await screen.findByTestId('health-explain-line')
    expect(line).toHaveTextContent('/healthz')
    expect(line).toHaveTextContent('Restarts: 4')
    expect(line).toHaveTextContent('Last error: boom')
    expect(line).toHaveTextContent('Roll back to the previous deploy')
  })

  it('renders nothing when healthy', async () => {
    stubExplain({
      app_name: 'api',
      state: 'healthy',
      crashloop: false,
      next_step: { kind: 'none' },
      computed_at: '2026-11-21T10:05:00Z',
    })
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByTestId('health-explain-line')).toBeNull()
  })
})
