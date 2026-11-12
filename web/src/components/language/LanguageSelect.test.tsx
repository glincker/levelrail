import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { LanguageSelect } from './LanguageSelect'
import { makeI18n, makeWrap } from './languageTestKit'

describe('LanguageSelect', () => {
  it('lists native names with codes from the supported list and marks the current one', () => {
    const i18n = makeI18n('es')
    render(<LanguageSelect onChange={vi.fn()} />, { wrapper: makeWrap(i18n) })
    const select = screen.getByRole('combobox')
    expect(select).toHaveValue('es')
    const names = screen.getAllByRole('option').map((o) => o.textContent)
    expect(names).toEqual(['English (en)', 'español (es)'])
  })

  it('maps a regional language onto the supported code', () => {
    const i18n = makeI18n('es-MX')
    render(<LanguageSelect onChange={vi.fn()} />, { wrapper: makeWrap(i18n) })
    expect(screen.getByRole('combobox')).toHaveValue('es')
  })

  it('reports the picked code', () => {
    const onChange = vi.fn()
    render(<LanguageSelect onChange={onChange} />, {
      wrapper: makeWrap(makeI18n()),
    })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'es' } })
    expect(onChange).toHaveBeenCalledWith('es')
  })

  it('offers the browser default only when asked', () => {
    render(<LanguageSelect value="" allowDefault onChange={vi.fn()} />, {
      wrapper: makeWrap(makeI18n()),
    })
    expect(screen.getByRole('option', { name: 'Browser default' })).toBeTruthy()
    expect(screen.getByRole('combobox')).toHaveValue('')
  })

  it('compact variant shows the code and an accessible name with the language', () => {
    render(<LanguageSelect variant="compact" onChange={vi.fn()} />, {
      wrapper: makeWrap(makeI18n('es')),
    })
    expect(screen.getByText('es')).toBeTruthy()
    expect(
      screen.getByRole('combobox', { name: /Cambiar idioma, actual: español/ }),
    ).toBeTruthy()
  })
})
