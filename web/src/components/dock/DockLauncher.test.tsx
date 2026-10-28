import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { DockLauncher } from './DockLauncher'

describe('DockLauncher', () => {
  it('hides itself on narrow screens while a menu is open', async () => {
    const i18n = createInstance()
    await i18n
      .use(initReactI18next)
      .init({ lng: 'en', resources: { en: { translation: {} } } })
    render(
      <I18nextProvider i18n={i18n}>
        <DockLauncher />
      </I18nextProvider>,
    )
    const button = screen.getByRole('button')
    expect(button.className).toContain(
      'max-md:in-[body:has([role=menu])]:invisible',
    )
  })
})
