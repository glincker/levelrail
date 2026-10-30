import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { ErrorTrackingSetupCard } from './ErrorTrackingSetupCard'
import commonEn from '../../locales/en/common.json'
import errorsEn from '../../locales/en/errors.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['errors', 'common'],
  defaultNS: 'errors',
  resources: { en: { errors: errorsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('ErrorTrackingSetupCard', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the DSN once after turning error tracking on', async () => {
    const endpoint = 'https://cp.example.com/api/v1/sentry/1'
    fetchMock.mockImplementation(
      (_url: string, init?: RequestInit): Promise<Response> =>
        Promise.resolve(
          init?.method === 'POST'
            ? json({
                enabled: true,
                endpoint,
                dsn: 'https://d2Vi.secret@cp.example.com/api/v1/sentry/1',
              })
            : json({ enabled: false, endpoint }),
        ),
    )
    const client = createTestQueryClient()
    const user = userEvent.setup()
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={client}>
          <ErrorTrackingSetupCard app="web" />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    await user.click(
      await screen.findByRole('button', { name: 'Turn on error tracking' }),
    )
    await user.click(await screen.findByRole('button', { name: 'Reveal' }))
    await waitFor(() => {
      expect(
        screen.getByText(
          'SENTRY_DSN=https://d2Vi.secret@cp.example.com/api/v1/sentry/1',
        ),
      ).toBeInTheDocument()
    })
    expect(screen.getByText(/shown once/)).toBeInTheDocument()
  })
})
