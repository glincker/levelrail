import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { MailDemoBadge } from './MailDemoBadge'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  interpolation: { escapeValue: false },
})

describe('MailDemoBadge', () => {
  it('shows an unmistakable DEMO DATA badge and a reset button', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={i18n}>
          <MailDemoBadge />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    expect(screen.getByText('DEMO DATA')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Reset demo' })).toBeTruthy()
  })
})
