import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import databasesEn from '../locales/en/databases.json'
import nodesEn from '../locales/en/nodes.json'
import trafficEn from '../locales/en/traffic.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['databases', 'traffic', 'nodes'],
  defaultNS: 'databases',
  resources: {
    en: { databases: databasesEn, traffic: trafficEn, nodes: nodesEn },
  },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/databases.json catalogue. */
export function DatabasesI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
