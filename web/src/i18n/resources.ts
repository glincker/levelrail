import type common from '../locales/en/common.json'
import type deploys from '../locales/en/deploys.json'
import type settings from '../locales/en/settings.json'
import type dashboard from '../locales/en/dashboard.json'
import type auditLog from '../locales/en/auditLog.json'
import type streams from '../locales/en/streams.json'
import type networkProxy from '../locales/en/networkProxy.json'
import type databases from '../locales/en/databases.json'
import type https from '../locales/en/https.json'
import type nodes from '../locales/en/nodes.json'
import type access from '../locales/en/access.json'
import type environments from '../locales/en/environments.json'
import type exposure from '../locales/en/exposure.json'
import type migration from '../locales/en/migration.json'
import type domains from '../locales/en/domains.json'
import type attention from '../locales/en/attention.json'
import type setup from '../locales/en/setup.json'
import type updates from '../locales/en/updates.json'
import type databaseAccess from '../locales/en/databaseAccess.json'
import type databaseUpgrades from '../locales/en/databaseUpgrades.json'
import type iam from '../locales/en/iam.json'
import type signIn from '../locales/en/signIn.json'
import type traffic from '../locales/en/traffic.json'
import type dns from '../locales/en/dns.json'
import type domainPolicies from '../locales/en/domainPolicies.json'
import type previews from '../locales/en/previews.json'

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
      nodes: typeof nodes
      access: typeof access
      environments: typeof environments
      exposure: typeof exposure
      migration: typeof migration
      domains: typeof domains
      attention: typeof attention
      setup: typeof setup
      updates: typeof updates
      databaseAccess: typeof databaseAccess
      databaseUpgrades: typeof databaseUpgrades
      iam: typeof iam
      signIn: typeof signIn
      traffic: typeof traffic
      dns: typeof dns
      domainPolicies: typeof domainPolicies
      previews: typeof previews
    }
  }
}
