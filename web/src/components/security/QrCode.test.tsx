import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../../locales/en/settings.json'
import { QrCode } from './QrCode'

const SECRET = 'JBSWY3DPEHPK3PXP'
const URI = `otpauth://totp/Levelrail:me?secret=${SECRET}&issuer=Levelrail`

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('QrCode', () => {
  it('renders a local svg without the secret in its label or any network call', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const { container } = render(
      <I18nextProvider i18n={testI18n}>
        <QrCode value={URI} />
      </I18nextProvider>,
    )
    const img = await screen.findByLabelText(
      'QR code for your authenticator app',
    )
    expect(img.tagName.toLowerCase()).toBe('svg')
    expect(img.getAttribute('role')).toBe('img')
    expect(img.getAttribute('aria-label')).not.toContain(SECRET)
    expect(container.innerHTML).not.toContain(SECRET)
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})
