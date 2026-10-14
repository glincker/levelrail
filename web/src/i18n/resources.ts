import type common from '../locales/en/common.json'
import type deploys from '../locales/en/deploys.json'
import type settings from '../locales/en/settings.json'
import type dashboard from '../locales/en/dashboard.json'
import type auditLog from '../locales/en/auditLog.json'
import type streams from '../locales/en/streams.json'
import type networkProxy from '../locales/en/networkProxy.json'
import type network from '../locales/en/network.json'
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
import type security from '../locales/en/security.json'
import type traffic from '../locales/en/traffic.json'
import type dns from '../locales/en/dns.json'
import type domainPolicies from '../locales/en/domainPolicies.json'
import type observability from '../locales/en/observability.json'
import type previews from '../locales/en/previews.json'
import type backups from '../locales/en/backups.json'
import type gitBuild from '../locales/en/gitBuild.json'
import type teams from '../locales/en/teams.json'
import type compose from '../locales/en/compose.json'
import type smartDeploy from '../locales/en/smartDeploy.json'
import type dataExport from '../locales/en/dataExport.json'
import type deploySafety from '../locales/en/deploySafety.json'
import type jobs from '../locales/en/jobs.json'
import type envEditor from '../locales/en/envEditor.json'
import type resourceAdvisor from '../locales/en/resourceAdvisor.json'
import type newApp from '../locales/en/newApp.json'
import type getStarted from '../locales/en/getStarted.json'
import type usage from '../locales/en/usage.json'
import type sso from '../locales/en/sso.json'

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
      network: typeof network
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
      security: typeof security
      traffic: typeof traffic
      dns: typeof dns
      domainPolicies: typeof domainPolicies
      observability: typeof observability
      previews: typeof previews
      backups: typeof backups
      gitBuild: typeof gitBuild
      teams: typeof teams
      compose: typeof compose
      smartDeploy: typeof smartDeploy
      dataExport: typeof dataExport
      deploySafety: typeof deploySafety
      jobs: typeof jobs
      envEditor: typeof envEditor
      resourceAdvisor: typeof resourceAdvisor
      newApp: typeof newApp
      getStarted: typeof getStarted
      usage: typeof usage
      sso: typeof sso
    }
  }
}
