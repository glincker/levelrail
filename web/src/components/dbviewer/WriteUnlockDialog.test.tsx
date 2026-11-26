import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import stripEn from '../../locales/en/dbExplorerStrip.json'
import { WriteUnlockDialog } from './WriteUnlockDialog'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dbExplorerStrip'],
  defaultNS: 'dbExplorerStrip',
  resources: { en: { dbExplorerStrip: stripEn } },
  interpolation: { escapeValue: false },
})

describe('WriteUnlockDialog', () => {
  it('enables the unlock only once the database name is typed', () => {
    const onUnlock = vi.fn()
    render(
      <I18nextProvider i18n={testI18n}>
        <WriteUnlockDialog
          databaseName="postgres"
          open
          onOpenChange={() => undefined}
          onUnlock={onUnlock}
        />
      </I18nextProvider>,
    )
    const confirm = screen.getByRole('button', { name: 'Unlock writes' })
    expect(confirm).toBeDisabled()
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'postgre' } })
    expect(confirm).toBeDisabled()
    fireEvent.change(input, { target: { value: 'postgres' } })
    expect(confirm).toBeEnabled()
    fireEvent.click(confirm)
    expect(onUnlock).toHaveBeenCalledWith(15)
  })
})
