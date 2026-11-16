import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { LanguageMenu } from './LanguageMenu'
import { makeI18n, makeWrap } from './languageTestKit'

describe('LanguageMenu', () => {
  it('shows the current code and lists native and English names', async () => {
    render(<LanguageMenu value="en" onChange={vi.fn()} />, {
      wrapper: makeWrap(makeI18n()),
    })
    const trigger = screen.getByRole('button', { name: /English/ })
    expect(trigger).toHaveTextContent('en')
    await userEvent.click(trigger)
    const spanish = await screen.findByRole('menuitem', { name: /Español/ })
    expect(spanish).toHaveTextContent('Spanish')
    expect(screen.getByText('Language')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /English/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
  })

  it('calls onChange with the chosen code', async () => {
    const onChange = vi.fn()
    render(<LanguageMenu value="en" onChange={onChange} />, {
      wrapper: makeWrap(makeI18n()),
    })
    await userEvent.click(screen.getByRole('button', { name: /English/ }))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: /Español/ }),
    )
    expect(onChange).toHaveBeenCalledWith('es')
  })

  it('offers the browser language when it differs', async () => {
    vi.spyOn(window.navigator, 'language', 'get').mockReturnValue('es-MX')
    const onChange = vi.fn()
    render(<LanguageMenu value="en" onChange={onChange} />, {
      wrapper: makeWrap(makeI18n()),
    })
    await userEvent.click(screen.getByRole('button', { name: /English/ }))
    await userEvent.click(
      await screen.findByRole('menuitem', { name: /Use browser language/ }),
    )
    expect(onChange).toHaveBeenCalledWith('es')
    vi.restoreAllMocks()
  })
})
