import type { AnchorHTMLAttributes, ReactElement, ReactNode } from 'react'
import { render } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import dashboardsEn from '@/locales/en/dashboards.json'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dashboards'],
  defaultNS: 'dashboards',
  resources: { en: { dashboards: dashboardsEn } },
  interpolation: { escapeValue: false },
})

export function renderWidget(ui: ReactElement) {
  return render(<I18nextProvider i18n={i18n}>{ui}</I18nextProvider>)
}

/** Replaces the router Link with a plain anchor so widgets render without a router. */
export function routerMock() {
  return Promise.resolve({
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  })
}
