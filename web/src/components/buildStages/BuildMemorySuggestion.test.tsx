import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import buildMemoryEn from '../../locales/en/buildMemory.json'
import { BuildMemorySuggestion } from './BuildMemorySuggestion'

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['buildMemory'],
  defaultNS: 'buildMemory',
  resources: { en: { buildMemory: buildMemoryEn } },
  interpolation: { escapeValue: false },
})

function mockFetch(suggestion: Record<string, unknown> | undefined) {
  const calls: { url: string; body: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, body: typeof init?.body === 'string' ? init.body : '' })
      return Promise.resolve(
        new Response(
          JSON.stringify(
            init?.method === 'PUT'
              ? { effective: {}, global: {}, overrides: {} }
              : { runs: [], memory_suggestion: suggestion },
          ),
        ),
      )
    }),
  )
  return calls
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <BuildMemorySuggestion appName="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('BuildMemorySuggestion', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('sets the per-app build memory to the suggested value in one click', async () => {
    const calls = mockFetch({
      peak_mb: 1900,
      limit_mb: 2048,
      suggested_mb: 2560,
      route_to_node: false,
    })
    renderCard()
    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', {
        name: "Set this app's build memory to 2560 MiB",
      }),
    )
    const put = calls.find((c) => c.body !== '')
    expect(put?.url).toBe('/api/v1/apps/web/build-limits')
    expect(JSON.parse(put?.body ?? '{}')).toEqual({ memory_mb: 2560 })
  })

  it('points to build nodes when the cap would not fit the host', async () => {
    mockFetch({
      peak_mb: 3000,
      limit_mb: 3000,
      suggested_mb: 3584,
      route_to_node: true,
    })
    renderCard()
    const link = await screen.findByRole('link', {
      name: 'Choose a build node',
    })
    expect(link.getAttribute('href')).toBe('/nodes')
  })

  it('renders nothing without a suggestion', async () => {
    mockFetch(undefined)
    renderCard()
    await new Promise((r) => setTimeout(r, 20))
    expect(screen.queryByTestId('build-memory-suggestion')).toBeNull()
  })
})
