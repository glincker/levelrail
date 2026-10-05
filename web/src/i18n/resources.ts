import type common from '../locales/en/common.json'
import type deploys from '../locales/en/deploys.json'
import type settings from '../locales/en/settings.json'
import type dashboard from '../locales/en/dashboard.json'
import type auditLog from '../locales/en/auditLog.json'
import type streams from '../locales/en/streams.json'
import type networkProxy from '../locales/en/networkProxy.json'
import type databases from '../locales/en/databases.json'
import type https from '../locales/en/https.json'

// Makes a typo'd translation key a tsc error instead of a silent runtime fallback.
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'common'
    resources: {
      common: typeof common
      deploys: typeof deploys
      settings: typeof settings
      dashboard: typeof dashboard
      auditLog: typeof auditLog
      streams: typeof streams
      networkProxy: typeof networkProxy
      databases: typeof databases
      https: typeof https
    }
  }
}
