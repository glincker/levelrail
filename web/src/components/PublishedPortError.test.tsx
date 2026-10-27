import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { PublishedPortError } from './PublishedPortError'
import composeEn from '../locales/en/compose.json'
import { ApiError } from '../lib/apiError'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['compose'],
  defaultNS: 'compose',
  resources: { en: { compose: composeEn } },
  interpolation: { escapeValue: false },
})

function renderError(error: Error) {
  return render(
    <I18nextProvider i18n={testI18n}>
      <PublishedPortError error={error} />
    </I18nextProvider>,
  )
}

describe('PublishedPortError', () => {
  it('shows the server message and a hint for a conflict code', () => {
    const msg = 'host port 53/udp for service "dns" is already published by "x"'
    renderError(new ApiError(409, msg, { code: 'published_port_conflict' }))
    expect(screen.getByText(msg)).toBeInTheDocument()
    expect(screen.getByText(/Pick a free host port/)).toBeInTheDocument()
  })

  it('shows only the message for other errors', () => {
    renderError(new ApiError(500, 'boom'))
    expect(screen.getByText('boom')).toBeInTheDocument()
    expect(screen.queryByText(/host port/i)).not.toBeInTheDocument()
  })
})
