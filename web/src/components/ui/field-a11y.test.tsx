import { useState } from 'react'
import type { ReactElement } from 'react'
import { act, render as rtlRender, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { z } from 'zod'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import { useAppForm } from '../../hooks/useAppForm'
import { Field, FieldError, FieldHint, FieldLabel } from './field'
import { FormErrorSummary } from './form-error-summary'
import { Input } from './input'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
  interpolation: { escapeValue: false },
})

function render(ui: ReactElement) {
  return rtlRender(<I18nextProvider i18n={i18n}>{ui}</I18nextProvider>)
}

const scroll = vi.fn()
beforeEach(() => {
  scroll.mockClear()
  Element.prototype.scrollIntoView = scroll
})
afterEach(() => {
  vi.useRealTimers()
})

describe('Field wiring', () => {
  it('links label and control with a generated id', () => {
    render(
      <Field>
        <FieldLabel>Name</FieldLabel>
        <Input />
      </Field>,
    )
    const input = screen.getByLabelText('Name')
    expect(input.id).not.toBe('')
    expect(screen.getByText('Name')).toHaveAttribute('for', input.id)
  })

  it('keeps an explicit id and gives two fields distinct ids', () => {
    render(
      <>
        <Field>
          <FieldLabel htmlFor="mine">One</FieldLabel>
          <Input id="mine" />
        </Field>
        <Field>
          <FieldLabel>Two</FieldLabel>
          <Input />
        </Field>
      </>,
    )
    expect(screen.getByLabelText('One').id).toBe('mine')
    expect(screen.getByLabelText('Two').id).not.toBe('mine')
  })

  it('toggles aria-invalid with the error and points describedby at hint and error', () => {
    function Demo({ error }: { error: boolean }) {
      return (
        <Field>
          <FieldLabel>Port</FieldLabel>
          <Input />
          <FieldHint>A number</FieldHint>
          {error ? <FieldError>Required</FieldError> : null}
        </Field>
      )
    }
    const { rerender } = render(<Demo error={false} />)
    const input = screen.getByLabelText('Port')
    expect(input).not.toHaveAttribute('aria-invalid')
    expect(input).toHaveAttribute(
      'aria-describedby',
      screen.getByText('A number').id,
    )
    rerender(
      <I18nextProvider i18n={i18n}>
        <Demo error />
      </I18nextProvider>,
    )
    expect(input).toHaveAttribute('aria-invalid', 'true')
    const ids = (input.getAttribute('aria-describedby') ?? '').split(' ')
    expect(ids).toEqual([
      screen.getByText('A number').id,
      screen.getByText('Required').id,
    ])
  })

  it('sets aria-required and honours data-invalid', () => {
    render(
      <Field required data-invalid>
        <FieldLabel>Image</FieldLabel>
        <Input />
      </Field>,
    )
    const input = screen.getByLabelText('Image')
    expect(input).toHaveAttribute('aria-required', 'true')
    expect(input).toHaveAttribute('aria-invalid', 'true')
  })

  it('announces an error with role=alert only when it first appears', () => {
    vi.useFakeTimers()
    render(
      <Field>
        <FieldLabel>Name</FieldLabel>
        <Input />
        <FieldError>Bad</FieldError>
      </Field>,
    )
    expect(screen.getAllByRole('alert')).toHaveLength(1)
    act(() => {
      vi.advanceTimersByTime(1200)
    })
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByText('Bad')).toBeInTheDocument()
  })
})

const schema = z.object({
  name: z.string().min(1, 'Name is required'),
  image: z.string().min(1, 'Image is required'),
})

function Form() {
  const form = useAppForm(schema, { defaultValues: { name: '', image: '' } })
  const { errors } = form.formState
  return (
    <form
      noValidate
      onSubmit={(e) => {
        void form.handleSubmit(() => undefined)(e)
      }}
    >
      <FormErrorSummary
        form={form}
        fields={{
          name: { label: 'Name label', id: 'f-name' },
          image: { label: 'Image label' },
        }}
      />
      <Field>
        <FieldLabel>Name</FieldLabel>
        <Input id="f-name" {...form.register('name')} />
        <FieldError errors={[errors.name]} />
      </Field>
      <Field>
        <FieldLabel>Image</FieldLabel>
        <Input {...form.register('image')} />
        <FieldError errors={[errors.image]} />
      </Field>
      <button type="submit">Save</button>
    </form>
  )
}

describe('Form submit failure', () => {
  it('focuses the first invalid field, lists the summary and links focus fields', async () => {
    const user = userEvent.setup()
    render(<Form />)
    expect(screen.queryByRole('link')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Save' }))

    const name = screen.getByLabelText('Name')
    const image = screen.getByLabelText('Image')
    await vi.waitFor(() => {
      expect(name).toHaveFocus()
    })
    expect(name).toHaveAttribute('aria-invalid', 'true')
    expect(image).toHaveAttribute('aria-invalid', 'true')
    await vi.waitFor(() => {
      expect(scroll).toHaveBeenCalled()
    })

    const status = screen.getByRole('status')
    expect(status).toHaveTextContent('2 fields need attention')
    const links = screen.getAllByRole('link')
    expect(links.map((l) => l.textContent)).toEqual([
      'Name label',
      'Image label',
    ])

    await user.click(links[1] as HTMLElement)
    expect(image).toHaveFocus()
  })

  it('does not stack extra role=alert elements beyond one per invalid field', async () => {
    const user = userEvent.setup()
    render(<Form />)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(screen.getAllByRole('alert')).toHaveLength(2)
    expect(screen.getByRole('status')).not.toHaveAttribute('role', 'alert')
  })
})

describe('FieldError in a stateful form', () => {
  it('works with controlled inputs', async () => {
    const user = userEvent.setup()
    function Controlled() {
      const [v, setV] = useState('')
      return (
        <Field>
          <FieldLabel>Host</FieldLabel>
          <Input
            value={v}
            onChange={(e) => {
              setV(e.target.value)
            }}
          />
          {v === '' ? <FieldError>Empty</FieldError> : null}
        </Field>
      )
    }
    render(<Controlled />)
    const input = screen.getByLabelText('Host')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    await user.type(input, 'x')
    expect(input).not.toHaveAttribute('aria-invalid')
  })
})
