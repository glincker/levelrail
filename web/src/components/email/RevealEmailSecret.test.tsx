import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RevealEmailSecret, REVEAL_HIDE_AFTER_MS } from './RevealEmailSecret'
import settingsEn from '../../locales/en/settings.json'
import envEditorEn from '../../locales/en/envEditor.json'

const SECRET = 'prov-secret-AAAA-do-not-cache'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['settings', 'envEditor'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, envEditor: envEditorEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function setup(responses: Response[]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const fetchMock = vi.fn(() => {
    const next = responses.shift()
    return Promise.resolve(next ?? json({ methods: ['password'] }))
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  const view = render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <RevealEmailSecret providerId="prov_1" field="resend_api_key" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return { queryClient, user, view, fetchMock }
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('RevealEmailSecret', () => {
  it('shows the value, hides it after 30 seconds and never caches it', async () => {
    const { queryClient, user } = setup([json({ value: SECRET })])
    await user.click(screen.getByRole('button', { name: 'Reveal' }))
    expect(await screen.findByText(SECRET)).toBeTruthy()
    expect(screen.getByRole('button', { name: /copy/i })).toBeTruthy()

    const cached = JSON.stringify([
      queryClient
        .getQueryCache()
        .getAll()
        .map((q) => q.state.data),
      queryClient
        .getMutationCache()
        .getAll()
        .map((m) => m.state.data),
    ])
    expect(cached).not.toContain(SECRET)
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0)

    act(() => {
      vi.advanceTimersByTime(REVEAL_HIDE_AFTER_MS + 1)
    })
    expect(screen.queryByText(SECRET)).toBeNull()
    expect(screen.getByRole('button', { name: 'Reveal' })).toBeTruthy()
  })

  it('clears the value and its timer on unmount', async () => {
    const { user, view } = setup([json({ value: SECRET })])
    await user.click(screen.getByRole('button', { name: 'Reveal' }))
    expect(await screen.findByText(SECRET)).toBeTruthy()
    const clear = vi.spyOn(window, 'clearTimeout')
    view.unmount()
    expect(clear).toHaveBeenCalled()
    expect(document.body.textContent).not.toContain(SECRET)
  })

  it('explains that two-factor is required instead of showing a value', async () => {
    const { user } = setup([
      json({ error: 'needs 2fa', code: 'two_factor_required' }, 403),
    ])
    await user.click(screen.getByRole('button', { name: 'Reveal' }))
    expect(
      await screen.findByText(/needs two-factor authentication/i),
    ).toBeTruthy()
    expect(screen.queryByTestId('revealed-email-secret')).toBeNull()
  })

  it('opens the step-up dialog when the server asks for it', async () => {
    const { user } = setup([
      json({ error: 'confirm', code: 'step_up_required' }, 403),
    ])
    await user.click(screen.getByRole('button', { name: 'Reveal' }))
    expect(
      await screen.findByText(/authenticator code to reveal this credential/i),
    ).toBeTruthy()
  })
})
