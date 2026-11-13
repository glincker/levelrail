import { QueryClient } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { accountSelfKeys, type Preferences } from '../../queries/accountSelf'
import { HeaderLanguageSelect } from './HeaderLanguageSelect'
import { makeI18n, makeWrap } from './languageTestKit'

const saved: Preferences = {
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
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

function client(withPrefs: boolean) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  if (withPrefs) qc.setQueryData(accountSelfKeys.preferences, saved)
  else fetchMock.mockResolvedValue(new Response('{}', { status: 401 }))
  return qc
}

describe('HeaderLanguageSelect', () => {
  it('signed in: retranslates, saves the preference and updates the cache', async () => {
    fetchMock.mockImplementation((_url: string, init?: RequestInit) =>
      Promise.resolve(
        new Response(typeof init?.body === 'string' ? init.body : '{}', {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )
    const i18n = makeI18n()
    const qc = client(true)
    render(<HeaderLanguageSelect />, { wrapper: makeWrap(i18n, qc) })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'es' } })
    await waitFor(() => expect(i18n.language).toBe('es'))
    await waitFor(() =>
      expect(
        qc.getQueryData<Preferences>(accountSelfKeys.preferences)?.language,
      ).toBe('es'),
    )
    const [url, init] =
      fetchMock.mock.calls.find(([, i]) => i?.method === 'PUT') ?? []
    expect(url).toBe('/api/v1/auth/preferences')
    expect(
      JSON.parse(typeof init?.body === 'string' ? init.body : ''),
    ).toMatchObject({ language: 'es' })
    expect(window.localStorage.getItem('levelrail-language-chosen')).toBeNull()
  })

  it('signed out: applies the language and remembers it for after sign-in', async () => {
    const i18n = makeI18n()
    render(<HeaderLanguageSelect />, { wrapper: makeWrap(i18n, client(false)) })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'es' } })
    await waitFor(() => expect(i18n.language).toBe('es'))
    await waitFor(() =>
      expect(window.localStorage.getItem('levelrail-language-chosen')).toBe(
        'es',
      ),
    )
    expect(fetchMock.mock.calls.every(([, i]) => i?.method !== 'PUT')).toBe(
      true,
    )
  })

  it('two selects on one i18n instance stay in sync', async () => {
    const i18n = makeI18n()
    const Wrap = makeWrap(i18n, client(false))
    render(
      <>
        <HeaderLanguageSelect />
        <HeaderLanguageSelect />
      </>,
      { wrapper: Wrap },
    )
    const [a, b] = screen.getAllByRole('combobox')
    fireEvent.change(a as HTMLElement, { target: { value: 'es' } })
    await waitFor(() => expect(b).toHaveValue('es'))
  })
})
