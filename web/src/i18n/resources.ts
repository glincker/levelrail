import type common from '../locales/en/common.json'
import type deploys from '../locales/en/deploys.json'
import type settings from '../locales/en/settings.json'

// Makes a typo'd translation key a tsc error instead of a silent runtime fallback.
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'common'
    resources: {
      common: typeof common
      deploys: typeof deploys
      settings: typeof settings
    }
  }
}
