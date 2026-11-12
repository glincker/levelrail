import { QueryClient } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { makeI18n, makeWrap } from '../language/languageTestKit'
import { accountSelfKeys, type Preferences } from '../../queries/accountSelf'

vi.mock('@tanstack/react-router', () => ({
  useRouter: () => ({
    state: { location: { pathname: '/x' } },
    history: { replace: vi.fn() },
  }),
}))
vi.mock('../ThemeProvider', () => ({
  useTheme: () => ({ theme: 'system', setTheme: vi.fn() }),
}))

import { PreferencesApplier } from './PreferencesApplier'

const base: Preferences = {
  timezone: '',
  language: '',
  theme: '',
  landing_page: '',
  gravatar: false,
}
const fetchMock =
  vi.fn<(url: string, init?: RequestInit) => Promise<Response>>()

beforeEach(() => {
  window.localStorage.clear()
  window.sessionStorage.setItem('levelrail-landing-applied', '1')
  fetchMock.mockReset()
  fetchMock.mockImplementation((_u: string, init?: RequestInit) =>
    Promise.resolve(
      new Response(typeof init?.body === 'string' ? init.body : '{}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
})

function mount(prefs: Preferences) {
  const i18n = makeI18n('en')
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  qc.setQueryData(accountSelfKeys.preferences, prefs)
  const Wrap = makeWrap(i18n, qc)
  render(<PreferencesApplier />, { wrapper: Wrap })
  return i18n
}

describe('PreferencesApplier language', () => {
  it('applies the saved account language', async () => {
    const i18n = mount({ ...base, language: 'es' })
    await waitFor(() => expect(i18n.language).toBe('es'))
  })

  it('pushes a language chosen before sign-in to the account once', async () => {
    window.localStorage.setItem('levelrail-language-chosen', 'es')
    const i18n = mount(base)
    await waitFor(() => expect(i18n.language).toBe('es'))
    await waitFor(() => {
      const put = fetchMock.mock.calls.find(([, i]) => i?.method === 'PUT')
      expect(
        JSON.parse(typeof put?.[1]?.body === 'string' ? put[1].body : ''),
      ).toMatchObject({
        language: 'es',
      })
    })
    expect(window.localStorage.getItem('levelrail-language-chosen')).toBeNull()
  })

  it('keeps the browser language when nothing was chosen or saved', async () => {
    const i18n = mount(base)
    await new Promise((r) => setTimeout(r, 20))
    expect(i18n.language).toBe('en')
    expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'PUT')).toBe(
      false,
    )
  })
})
