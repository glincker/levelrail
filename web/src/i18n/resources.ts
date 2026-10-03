import type common from '../locales/en/common.json'
import type deploys from '../locales/en/deploys.json'

// The `en` JSON files are the single source of truth for every
// translation key: this augmentation makes a typo'd or missing key a
// tsc error at the t() call site instead of a silent runtime fallback
// to the raw key string.
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'common'
    resources: {
      common: typeof common
      deploys: typeof deploys
    }
  }
}
