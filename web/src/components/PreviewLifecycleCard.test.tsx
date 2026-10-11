import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import previewsEn from '../locales/en/previews.json'
import { PreviewLifecycleCard } from './PreviewLifecycleCard'
import type { PreviewPolicy } from '../types/previewEnvironment'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['previews'],
  defaultNS: 'previews',
  resources: { en: { previews: previewsEn } },
  interpolation: { escapeValue: false },
})

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

const basePolicy: PreviewPolicy = {
  on_limit: 'evict_oldest',
  allow_fork_previews: false,
  ttl_hours: 0,
  effective_ttl_hours: 168,
  max_per_app: 10,
  live_count: 0,
  max_total: 0,
  live_total: 0,
  max_previews: 0,
  memory_limit: '',
  cpu_limit: 0,
  effective_memory: '256Mi',
  effective_cpu: 0.25,
  idle_sleep_minutes: 0,
  effective_idle_sleep_minutes: 30,
  database_strategy: 'none',
  seed_database: '',
  allow_fork_secrets: false,
  gate_basic_auth: false,
  gate_username: '',
  gate_password_set: false,
  allow_indexing: false,
}

describe('PreviewLifecycleCard', () => {
  let puts: Record<string, unknown>[]

  beforeEach(() => {
    puts = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        if (url === '/api/v1/apps/web/git-source') {
          return Promise.resolve(
            jsonResponse({ app_name: 'web', preview_enabled: true }),
          )
        }
        if (url === '/api/v1/apps/web/preview-policy') {
          if (init?.method === 'PUT') {
            puts.push(
              JSON.parse(init.body as string) as Record<string, unknown>,
            )
          }
          return Promise.resolve(jsonResponse(basePolicy))
        }
        return Promise.resolve(jsonResponse({ error: 'unexpected' }, 500))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function renderCard() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={queryClient}>
          <PreviewLifecycleCard appName="web" />
        </QueryClientProvider>
      </I18nextProvider>,
    )
  }

  it('shows the safe defaults and saves a changed memory limit', async () => {
    const user = userEvent.setup()
    renderCard()

    const memory = await screen.findByLabelText('Memory per preview')
    expect(memory).toHaveAttribute('placeholder', '256Mi')
    expect(
      screen.getByRole('switch', {
        name: 'Let search engines index preview URLs',
      }),
    ).not.toBeChecked()

    await user.type(memory, '512Mi')
    await user.click(screen.getByRole('button', { name: 'Save settings' }))

    await waitFor(() => {
      expect(puts).toHaveLength(1)
    })
    expect(puts[0]).toMatchObject({
      memory_limit: '512Mi',
      database_strategy: 'none',
      allow_indexing: false,
      allow_fork_secrets: false,
      gate_basic_auth: false,
    })
    expect(puts[0]).not.toHaveProperty('gate_password')
  })

  it('needs a username and password before the gate can be saved', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(
      await screen.findByRole('switch', {
        name: 'Require basic auth on previews',
      }),
    )
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled()

    await user.type(screen.getByLabelText('Username'), 'reviewer')
    await user.type(screen.getByLabelText('Password'), 's3cret')
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeEnabled()
  })
})
