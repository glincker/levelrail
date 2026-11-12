import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { makeI18n, makeWrap } from '../language/languageTestKit'
import { accountSelfKeys } from '../../queries/accountSelf'
import { LanguageStep } from './LanguageStep'

function setup(lng: string) {
  const i18n = makeI18n(lng)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  qc.setQueryData(accountSelfKeys.preferences, {})
  return { i18n, Wrap: makeWrap(i18n, qc) }
}

describe('LanguageStep', () => {
  it('preselects the detected language', () => {
    const { Wrap } = setup('es-MX')
    render(<LanguageStep onAccept={vi.fn()} />, { wrapper: Wrap })
    expect(screen.getByRole('combobox', { name: 'Idioma' })).toHaveValue('es')
  })

  it('previews the choice without saving', async () => {
    const { i18n, Wrap } = setup('en')
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    render(<LanguageStep onAccept={vi.fn()} />, { wrapper: Wrap })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'es' } })
    await waitFor(() => expect(i18n.language).toBe('es'))
    expect(await screen.findByText('Idioma')).toBeTruthy()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('Enter accepts the default', () => {
    const onAccept = vi.fn()
    const { Wrap } = setup('en')
    render(<LanguageStep onAccept={onAccept} />, { wrapper: Wrap })
    fireEvent.keyDown(screen.getByRole('combobox'), { key: 'Enter' })
    expect(onAccept).toHaveBeenCalledOnce()
  })
})
