import type common from '../locales/en/common.json'
import type deploys from '../locales/en/deploys.json'
import type auditLog from '../locales/en/auditLog.json'
import type streams from '../locales/en/streams.json'
import type networkProxy from '../locales/en/networkProxy.json'

// Makes a typo'd translation key a tsc error instead of a silent runtime fallback.
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'common'
    resources: {
      common: typeof common
      deploys: typeof deploys
      auditLog: typeof auditLog
      streams: typeof streams
      networkProxy: typeof networkProxy
    }
  }
}
