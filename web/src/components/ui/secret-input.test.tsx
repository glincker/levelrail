import { useState } from 'react'
import type { ReactElement } from 'react'
import { render as rtlRender, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import { Field, FieldLabel } from './field'
import { SecretInput } from './secret-input'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

function render(ui: ReactElement) {
  const wrap = (el: ReactElement) => (
    <I18nextProvider i18n={i18n}>{el}</I18nextProvider>
  )
  const r = rtlRender(wrap(ui))
  return { ...r, rerender: (el: ReactElement) => r.rerender(wrap(el)) }
}

describe('SecretInput', () => {
  it('is hidden by default with new-password autocomplete', () => {
    render(<SecretInput aria-label="Token" />)
    const input = screen.getByLabelText('Token')
    expect(input).toHaveAttribute('type', 'password')
    expect(input).toHaveAttribute('autocomplete', 'new-password')
    expect(screen.getByRole('button', { name: 'Show value' })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })

  it('lets the caller override autocomplete', () => {
    render(<SecretInput aria-label="Token" autoComplete="off" />)
    expect(screen.getByLabelText('Token')).toHaveAttribute(
      'autocomplete',
      'off',
    )
  })

  it('toggles reveal and aria-pressed', async () => {
    render(<SecretInput aria-label="Token" />)
    await userEvent.click(screen.getByRole('button', { name: 'Show value' }))
    expect(screen.getByLabelText('Token')).toHaveAttribute('type', 'text')
    const hide = screen.getByRole('button', { name: 'Hide value' })
    expect(hide).toHaveAttribute('aria-pressed', 'true')
    await userEvent.click(hide)
    expect(screen.getByLabelText('Token')).toHaveAttribute('type', 'password')
  })

  it('shows the masked placeholder only when hasExistingValue', () => {
    const { rerender } = render(<SecretInput aria-label="Token" />)
    expect(screen.getByLabelText('Token')).not.toHaveAttribute('placeholder')
    rerender(<SecretInput aria-label="Token" hasExistingValue />)
    expect(screen.getByLabelText('Token')).toHaveAttribute(
      'placeholder',
      'Set. Leave blank to keep the current value.',
    )
  })

  it('hides again when focus leaves the control but not on toggle click', async () => {
    render(
      <>
        <SecretInput aria-label="Token" />
        <button type="button">other</button>
      </>,
    )
    await userEvent.click(screen.getByLabelText('Token'))
    await userEvent.click(screen.getByRole('button', { name: 'Show value' }))
    expect(screen.getByLabelText('Token')).toHaveAttribute('type', 'text')
    await userEvent.click(screen.getByRole('button', { name: 'other' }))
    expect(screen.getByLabelText('Token')).toHaveAttribute('type', 'password')
  })

  it('starts hidden again after remount', async () => {
    const { unmount } = render(<SecretInput aria-label="Token" />)
    await userEvent.click(screen.getByRole('button', { name: 'Show value' }))
    unmount()
    render(<SecretInput aria-label="Token" />)
    expect(screen.getByLabelText('Token')).toHaveAttribute('type', 'password')
  })

  it('forwards ref and native props', () => {
    const ref = vi.fn()
    const onChange = vi.fn()
    render(
      <SecretInput
        aria-label="Token"
        ref={ref}
        name="tok"
        onChange={onChange}
      />,
    )
    expect(ref).toHaveBeenCalledWith(expect.any(HTMLInputElement))
    expect(screen.getByLabelText('Token')).toHaveAttribute('name', 'tok')
  })

  it('works with Field and react-hook-form register', async () => {
    const onSubmit = vi.fn()
    function Form() {
      const { register, handleSubmit } = useForm<{ token: string }>({
        defaultValues: { token: '' },
      })
      return (
        <form
          onSubmit={(e) => {
            void handleSubmit(onSubmit)(e)
          }}
        >
          <Field>
            <FieldLabel htmlFor="tok">Token</FieldLabel>
            <SecretInput id="tok" {...register('token')} />
          </Field>
          <button type="submit">save</button>
        </form>
      )
    }
    render(<Form />)
    await userEvent.type(screen.getByLabelText('Token'), 'abc')
    await userEvent.click(screen.getByRole('button', { name: 'save' }))
    expect(onSubmit).toHaveBeenCalledWith({ token: 'abc' }, expect.anything())
  })

  it('works controlled', async () => {
    function C() {
      const [v, setV] = useState('')
      return (
        <SecretInput
          aria-label="Token"
          value={v}
          onChange={(e) => {
            setV(e.target.value)
          }}
        />
      )
    }
    render(<C />)
    await userEvent.type(screen.getByLabelText('Token'), 'xy')
    expect(screen.getByLabelText('Token')).toHaveValue('xy')
  })
})
