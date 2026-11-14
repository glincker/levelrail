import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import iacFormEn from '../../locales/en/iacForm.json'
import { IacSchemaForm } from './IacSchemaForm'
import { realSchema } from './iacTestSchema'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['iacForm'],
  defaultNS: 'iacForm',
  resources: { en: { iacForm: iacFormEn } },
  interpolation: { escapeValue: false },
})

function renderForm(initial: string, onChange = vi.fn()) {
  function Bound() {
    const [v, setV] = useState(initial)
    return (
      <IacSchemaForm
        schema={realSchema}
        value={v}
        onChange={(y) => {
          onChange(y)
          setV(y)
        }}
      />
    )
  }
  return render(
    <I18nextProvider i18n={i18n}>
      <Bound />
    </I18nextProvider>,
  )
}

describe('IacSchemaForm', () => {
  it('renders the required name field and calls onChange with yaml', () => {
    const onChange = vi.fn()
    renderForm('', onChange)
    const name = screen.getByRole('textbox', { name: /^Name/ })
    fireEvent.change(name, { target: { value: 'shop' } })
    expect(onChange).toHaveBeenLastCalledWith(
      expect.stringContaining('name: shop'),
    )
    expect(onChange.mock.lastCall?.[0]).toContain('kind: App')
  })

  it('shows required hints and a beta badge for beta kinds', () => {
    const { rerender } = renderForm(
      'version: 1\nkind: Database\nmetadata:\n  name: db\n',
    )
    expect(screen.getByLabelText(/^Engine/)).toBeInTheDocument()
    expect(screen.getByText('Required.')).toBeInTheDocument()
    expect(screen.queryByText('Beta')).toBeNull()
    rerender(<div />)
  })

  it('never renders a secret value as plain text', () => {
    renderForm(
      'version: 1\nkind: App\nmetadata:\n  name: shop\nspec:\n  service:\n    image: nginx\n    env:\n      DB_PASSWORD:\n        secretRef: db-pass\n',
    )
    fireEvent.click(screen.getByText(/^Advanced/))
    const refInput = screen.getByDisplayValue('db-pass')
    expect(refInput).toHaveAttribute('placeholder', 'Secret name')
    expect(
      screen.getByRole('switch', { name: /secret reference for DB_PASSWORD/ }),
    ).toBeChecked()
  })

  it('locks the form on invalid yaml', () => {
    renderForm('a: 1\n  b: 2\n')
    expect(screen.getByRole('alert')).toHaveTextContent(
      /cannot be shown as a form/,
    )
  })
})
