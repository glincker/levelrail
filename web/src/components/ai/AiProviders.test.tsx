import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import settingsEn from '../../locales/en/settings.json'
import type { AiProvider } from '../../queries/aiProviders'
import { AiProviderCard } from './AiProviderCard'
import { AiProviderDrawer } from './AiProviderDrawer'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'common'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, common: {} } },
  interpolation: { escapeValue: false },
})

const saved: AiProvider = {
  id: 'openai',
  label: 'OpenAI',
  kind: 'openai_compat',
  brand: 'openai',
  has_key: true,
  status: 'ok',
  default_model: 'gpt-x',
  key_url: 'https://example.com/keys',
  default_base_url: '',
  base_url_editable: false,
}

function stubFetch(handler: (url: string, init?: RequestInit) => Response) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve(handler(url, init)),
    ),
  )
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function renderDrawer(provider: AiProvider, canWrite = true) {
  const qc = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <AiProviderDrawer
          provider={provider}
          onOpenChange={() => undefined}
          canWrite={canWrite}
          config={undefined}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AiProviderDrawer', () => {
  it('never prefills a saved key', async () => {
    stubFetch(() => json([]))
    renderDrawer(saved)
    const key = await screen.findByLabelText('API key')
    expect(key).toHaveValue('')
    expect(key).toHaveAttribute('type', 'password')
  })

  it('keeps Save disabled until valid', async () => {
    stubFetch(() => json([]))
    const user = userEvent.setup()
    renderDrawer({ ...saved, has_key: false })
    const save = await screen.findByRole('button', { name: 'Save' })
    expect(save).toBeDisabled()
    await user.type(screen.getByLabelText('API key'), 'sk-test')
    expect(save).toBeEnabled()
  })

  it('shows the 422 error and fix', async () => {
    stubFetch((_url, init) =>
      init?.method === 'PUT'
        ? json(
            { error: 'Key rejected', code: 'bad_key', fix: 'Check the key.' },
            422,
          )
        : json([]),
    )
    const user = userEvent.setup()
    renderDrawer({ ...saved, has_key: false })
    await user.type(await screen.findByLabelText('API key'), 'sk-test')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Key rejected')).toBeInTheDocument()
    expect(screen.getByText('Check the key.')).toBeInTheDocument()
  })

  it('asks before disconnecting', async () => {
    const calls: string[] = []
    stubFetch((url, init) => {
      calls.push(`${init?.method ?? 'GET'} ${url}`)
      return json([])
    })
    const user = userEvent.setup()
    renderDrawer(saved)
    await user.click(await screen.findByRole('button', { name: 'Disconnect' }))
    expect(calls.some((c) => c.startsWith('DELETE'))).toBe(false)
    expect(await screen.findByText('Disconnect OpenAI?')).toBeInTheDocument()
    const confirm = screen
      .getAllByRole('button', { name: 'Disconnect' })
      .at(-1)!
    await user.click(confirm)
    await waitFor(() => {
      expect(calls.some((c) => c.startsWith('DELETE'))).toBe(true)
    })
  })

  it('disables controls for non-root users', async () => {
    stubFetch(() => json([]))
    renderDrawer(saved, false)
    expect(await screen.findByLabelText('API key')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(screen.getByText(/Only the root account/)).toBeInTheDocument()
  })
})

describe('AiProviderCard', () => {
  function renderCard(p: AiProvider) {
    return render(
      <I18nextProvider i18n={testI18n}>
        <AiProviderCard provider={p} active={false} onOpen={() => undefined} />
      </I18nextProvider>,
    )
  }

  it('renders the brand mark when a brand is set', () => {
    const { container } = renderCard(saved)
    expect(
      container.querySelector('[data-slot="brand-tile"] svg'),
    ).not.toBeNull()
  })

  it('falls back to the monogram when brand is empty', () => {
    const { container } = renderCard({ ...saved, brand: '' })
    const tile = container.querySelector('[data-slot="brand-tile"]')
    expect(tile?.querySelector('svg')).toBeNull()
    expect(tile).toHaveTextContent('O')
  })

  it('shows a typed status pill', () => {
    renderCard({ ...saved, status: 'quota_exhausted' })
    expect(screen.getByText('Out of credits')).toBeInTheDocument()
  })
})
