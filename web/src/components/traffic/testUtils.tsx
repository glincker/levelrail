import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import trafficEn from '../../locales/en/traffic.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['traffic'],
  defaultNS: 'traffic',
  resources: { en: { traffic: trafficEn } },
  interpolation: { escapeValue: false },
})

export { testI18n }

export function renderTraffic(ui: ReactElement) {
  return render(<I18nextProvider i18n={testI18n}>{ui}</I18nextProvider>)
}
