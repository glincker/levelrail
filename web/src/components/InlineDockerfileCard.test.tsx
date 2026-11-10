import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { InlineDockerfileCard } from './InlineDockerfileCard'
import gitSourceEn from '../locales/en/gitSource.json'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="#">{children}</a>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['gitSource'],
  defaultNS: 'gitSource',
  resources: { en: { gitSource: gitSourceEn } },
  interpolation: { escapeValue: false },
})

function renderCard() {
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <InlineDockerfileCard appName="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('InlineDockerfileCard', () => {
  it('renders labelled controls', () => {
    renderCard()
    expect(screen.getByLabelText('Dockerfile')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Load from file' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Add context file' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Deploy' })).toBeDisabled()
  })

  it('posts the inline_source body', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ id: 'att-1' }), { status: 202 }),
      )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderCard()
    await user.type(screen.getByLabelText('Dockerfile'), 'FROM alpine')
    await user.click(screen.getByRole('button', { name: 'Add context file' }))
    await user.type(screen.getByLabelText('Context file 1 path'), 'a.txt')
    await user.type(screen.getByLabelText('Context file 1 content'), 'hi')
    await user.click(screen.getByRole('button', { name: 'Deploy' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/apps/web/builds')
    expect(JSON.parse(init.body as string)).toEqual({
      build: { type: 'dockerfile' },
      inline_source: {
        dockerfile: 'FROM alpine',
        files: [{ path: 'a.txt', content: 'hi' }],
      },
    })
    expect(await screen.findByText(/att-1/)).toBeInTheDocument()
  })

  it('shows the server message on 413', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: 'inline source too large' }), {
          status: 413,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )
    const user = userEvent.setup()
    renderCard()
    await user.type(screen.getByLabelText('Dockerfile'), 'FROM alpine')
    await user.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'inline source too large',
    )
  })
})
