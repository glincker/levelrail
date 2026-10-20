import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AiControlCard } from './AiControlCard'
import settingsEn from '../locales/en/settings.json'
import type { AiControlSettings } from '../queries/aiControl'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const base: AiControlSettings = {
  mode: 'observe',
  allowed_env_kinds: ['dev', 'test'],
  env_kinds: ['dev', 'test', 'uat', 'production', 'preview', 'custom'],
  admin_available: false,
  agent_token_count: 2,
  updated_at: '2026-10-06T10:00:00Z',
  updated_by: 'gagan',
}

function stubFetch(body: unknown) {
  const fn = vi.fn(() =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(body),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderCard(settings: AiControlSettings = base) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <AiControlCard settings={settings} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AiControlCard', () => {
  it('disables admin mode with an explanation when the flag is off', () => {
    renderCard()
    expect(screen.getByRole('radio', { name: /Admin/ })).toBeDisabled()
    expect(
      screen.getByText(/experimental feature ai-control/),
    ).toBeInTheDocument()
  })

  it('enables admin mode when available', () => {
    renderCard({ ...base, admin_available: true })
    expect(screen.getByRole('radio', { name: /Admin/ })).toBeEnabled()
  })

  it('saves the chosen mode and environment kinds', async () => {
    const fetchMock = stubFetch({ ...base, mode: 'operate' })
    renderCard()
    await userEvent.click(screen.getByRole('radio', { name: /Operate/ }))
    await userEvent.click(screen.getByRole('checkbox', { name: 'UAT' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(url).toBe('/api/v1/settings/ai-control')
    expect(init.method).toBe('PUT')
    expect(JSON.parse(init.body as string)).toEqual({
      mode: 'operate',
      allowed_env_kinds: ['dev', 'test', 'uat'],
    })
  })

  it('pauses all agents by setting mode off', async () => {
    const fetchMock = stubFetch({ ...base, mode: 'off' })
    renderCard()
    await userEvent.click(
      screen.getByRole('button', { name: /Pause all agents/ }),
    )
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toMatchObject({ mode: 'off' })
  })

  it('does not offer pause when already off', () => {
    renderCard({ ...base, mode: 'off' })
    expect(
      screen.getByRole('button', { name: /Pause all agents/ }),
    ).toBeDisabled()
  })

  it('revokes agent tokens only after confirmation', async () => {
    const fetchMock = stubFetch({ revoked: 2 })
    renderCard()
    expect(screen.getByText('2 agent tokens exist.')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Revoke all agent tokens' }),
    )
    expect(fetchMock).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: 'Revoke tokens' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(url).toBe('/api/v1/settings/ai-control/revoke-agent-tokens')
    expect(init.method).toBe('POST')
  })

  it('disables environment kinds in modes that ignore them', () => {
    renderCard()
    expect(screen.getByRole('checkbox', { name: 'Dev' })).toHaveAttribute(
      'aria-disabled',
      'true',
    )
  })
})
