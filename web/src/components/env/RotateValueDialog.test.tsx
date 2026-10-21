import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import secretRotationEn from '../../locales/en/secretRotation.json'
import { RotateValueDialog } from './RotateValueDialog'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common', 'secretRotation'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, secretRotation: secretRotationEn } },
  interpolation: { escapeValue: false },
})

function setup(
  props: Partial<React.ComponentProps<typeof RotateValueDialog>> = {},
) {
  const onSubmit = vi.fn()
  render(
    <I18nextProvider i18n={testI18n}>
      <RotateValueDialog
        open
        onOpenChange={() => undefined}
        title="Rotate API_KEY?"
        description="Write a new value now."
        consequences={['Running containers keep the old value.']}
        valueLabel="New value"
        pending={false}
        error={null}
        onSubmit={onSubmit}
        {...props}
      />
    </I18nextProvider>,
  )
  return { onSubmit }
}

describe('RotateValueDialog', () => {
  it('submits an empty value to ask for a generated one', async () => {
    const { onSubmit } = setup()
    await userEvent.click(
      screen.getByRole('button', { name: 'Generate and rotate' }),
    )
    expect(onSubmit).toHaveBeenCalledWith('')
  })

  it('submits the typed value through a masked field', async () => {
    const { onSubmit } = setup()
    const field = screen.getByLabelText('New value')
    expect(field).toHaveAttribute('type', 'password')
    await userEvent.type(field, 'next-value')
    await userEvent.click(screen.getByRole('button', { name: 'Rotate' }))
    expect(onSubmit).toHaveBeenCalledWith('next-value')
  })

  it('blocks confirm until a required value is typed', async () => {
    const { onSubmit } = setup({ valueRequired: true })
    const confirm = screen.getByRole('button', { name: 'Rotate' })
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText('New value'), 'x')
    expect(confirm).toBeEnabled()
    await userEvent.click(confirm)
    expect(onSubmit).toHaveBeenCalledWith('x')
  })

  it('shows the error from a failed rotation', () => {
    setup({ error: 'rotation failed' })
    expect(screen.getByText('rotation failed')).toBeInTheDocument()
  })
})
