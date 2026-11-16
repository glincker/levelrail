import { QueryClient } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { accountSelfKeys, type Preferences } from '../../queries/accountSelf'
import { HeaderLanguageSelect } from './HeaderLanguageSelect'
import { LanguageSelect } from './LanguageSelect'
import { makeI18n, makeWrap } from './languageTestKit'

const prefs: Preferences = {
  timezone: '',
  language: '',
  theme: '',
  landing_page: '',
  gravatar: false,
}

describe('language sync across controls', () => {
  it('a header change reaches a field select on the same i18n instance', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((_u: string, init?: RequestInit) =>
        Promise.resolve(
          new Response(typeof init?.body === 'string' ? init.body : '{}', {
            status: 200,
          }),
        ),
      ),
    )
    const i18n = makeI18n('en')
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    qc.setQueryData(accountSelfKeys.preferences, prefs)
    render(
      <>
        <HeaderLanguageSelect />
        <LanguageSelect onChange={vi.fn()} />
      </>,
      { wrapper: makeWrap(i18n, qc) },
    )
    await userEvent.click(screen.getByRole('button', { name: /English/ }))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: /Español/ }),
    )
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveValue('es'))
  })
})
