import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'

const modules = import.meta.glob<Record<string, unknown>>(
  '../locales/en/*.json',
  { eager: true, import: 'default' },
)

const resources: Record<string, Record<string, unknown>> = {}
for (const [path, json] of Object.entries(modules)) {
  const name = path.replace(/^.*\/(.+)\.json$/, '$1')
  resources[name] = json
}

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: Object.keys(resources),
  defaultNS: 'common',
  resources: { en: resources },
  interpolation: { escapeValue: false },
})

/** Renders children with every real en/*.json catalogue loaded. */
export function AllI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
