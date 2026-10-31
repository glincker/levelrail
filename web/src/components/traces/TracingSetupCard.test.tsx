import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { TracingSetupCard } from './TracingSetupCard'
import commonEn from '../../locales/en/common.json'
import tracesEn from '../../locales/en/traces.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['traces', 'common'],
  defaultNS: 'traces',
  resources: { en: { traces: tracesEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

function renderCard() {
  const client = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <TracingSetupCard app="web" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('TracingSetupCard', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the key and env once after turning tracing on', async () => {
    fetchMock.mockImplementation(
      (_url: string, init?: RequestInit): Promise<Response> =>
        Promise.resolve(
          init?.method === 'POST'
            ? json({
                enabled: true,
                endpoint: 'https://cp.example.com/api/v1/otlp',
                header: 'Authorization',
                key: 'd2Vi.secret',
                env: {
                  OTEL_EXPORTER_OTLP_ENDPOINT:
                    'https://cp.example.com/api/v1/otlp',
                  OTEL_EXPORTER_OTLP_HEADERS:
                    'Authorization=Bearer d2Vi.secret',
                },
              })
            : json({
                enabled: false,
                endpoint: 'https://cp.example.com/api/v1/otlp',
                header: 'Authorization',
              }),
        ),
    )
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('button', { name: 'Turn on tracing' }),
    )
    await user.click(await screen.findByRole('button', { name: 'Reveal' }))
    await waitFor(() => {
      expect(
        screen.getByText(
          /OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer d2Vi.secret/,
        ),
      ).toBeInTheDocument()
    })
    expect(screen.getByText(/shown once/)).toBeInTheDocument()
  })

  it('names the endpoint, not the key, when tracing was already on', async () => {
    fetchMock.mockResolvedValue(
      json({
        enabled: true,
        endpoint: 'https://cp.example.com/api/v1/otlp',
        header: 'Authorization',
      }),
    )
    renderCard()
    expect(
      await screen.findByText('https://cp.example.com/api/v1/otlp'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Rotate key' }),
    ).toBeInTheDocument()
  })
})
