import type { ReactNode } from 'react'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import accountEn from '../../locales/en/account.json'
import commonEn from '../../locales/en/common.json'
import type { Preferences } from '../../queries/accountSelf'

const mutate = vi.fn()
const setTheme = vi.fn()
let stored: Preferences

vi.mock('../../queries/accountSelf', () => ({
  preferencesQueryOptions: () => ({
    queryKey: ['prefs-test'],
    queryFn: () => Promise.resolve(stored),
  }),
  useSavePreferences: () => ({
    mutate,
    isPending: false,
    isSuccess: false,
  }),
}))
vi.mock('../ThemeProvider', () => ({
  useTheme: () => ({ theme: 'system', setTheme }),
}))

import { DEFAULT_PREFERENCES } from './preferenceDefaults'
import { PreferencesCard } from './PreferencesCard'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  supportedLngs: ['en', 'es', 'cimode'],
  ns: ['account', 'common'],
  defaultNS: 'account',
  resources: { en: { account: accountEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function Wrap({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}

async function renderCard(prefs: Preferences = DEFAULT_PREFERENCES) {
  stored = prefs
  render(<PreferencesCard />, { wrapper: Wrap })
  await screen.findByText('Preferences')
}

beforeEach(() => {
  mutate.mockClear()
  setTheme.mockClear()
})

describe('PreferencesCard', () => {
  it('filters timezones as you type and saves the picked zone', async () => {
    await renderCard()
    fireEvent.click(screen.getByRole('combobox', { name: /timezone/i }))
    const search = await screen.findByRole('searchbox')
    fireEvent.change(search, { target: { value: 'tokyo' } })
    expect(screen.queryByText('America/Chicago')).toBeNull()
    fireEvent.click(await screen.findByRole('option', { name: /Asia\/Tokyo/ }))
    expect(mutate).toHaveBeenCalledWith(
      { ...DEFAULT_PREFERENCES, timezone: 'Asia/Tokyo' },
      expect.anything(),
    )
  })

  it('lists the browser default first and marks the active language', async () => {
    await renderCard({ ...DEFAULT_PREFERENCES, language: 'es' })
    const select = screen.getByRole('combobox', { name: /language/i })
    const options = within(select).getAllByRole('option')
    expect(options[0]).toHaveTextContent('Browser default')
    expect(select).toHaveValue('es')
    fireEvent.change(select, { target: { value: 'en' } })
    expect(mutate).toHaveBeenCalledWith(
      { ...DEFAULT_PREFERENCES, language: 'en' },
      expect.anything(),
    )
  })

  it('applies a theme choice at once and saves it', async () => {
    await renderCard()
    fireEvent.click(screen.getByRole('radio', { name: 'Dark' }))
    expect(setTheme).toHaveBeenCalledWith('dark')
    expect(mutate).toHaveBeenCalledWith(
      { ...DEFAULT_PREFERENCES, theme: 'dark' },
      expect.anything(),
    )
  })

  it('resets to defaults, and disables reset when already default', async () => {
    await renderCard({ ...DEFAULT_PREFERENCES, theme: 'light', gravatar: true })
    fireEvent.click(screen.getByRole('button', { name: /reset to defaults/i }))
    expect(mutate).toHaveBeenCalledWith(DEFAULT_PREFERENCES, expect.anything())
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: /reset to defaults/i }),
      ).toBeDisabled(),
    )
  })
})
