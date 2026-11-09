import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import commonEn from '../locales/en/common.json'
import databasesEn from '../locales/en/databases.json'
import functionsEn from '../locales/en/functions.json'
import imagesEn from '../locales/en/images.json'
import appExpiryEn from '../locales/en/appExpiry.json'
import appRestartScheduleEn from '../locales/en/appRestartSchedule.json'
import storageBucketsEn from '../locales/en/storageBuckets.json'

export const newSurfacesI18n = i18next.createInstance()
void newSurfacesI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: [
    'common',
    'databases',
    'functions',
    'images',
    'storageBuckets',
    'appExpiry',
    'appRestartSchedule',
  ],
  defaultNS: 'common',
  resources: {
    en: {
      common: commonEn,
      databases: databasesEn,
      functions: functionsEn,
      images: imagesEn,
      appExpiry: appExpiryEn,
      appRestartSchedule: appRestartScheduleEn,
      storageBuckets: storageBucketsEn,
    },
  },
  interpolation: { escapeValue: false },
})

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}
