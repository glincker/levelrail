import type common from '../locales/en/common.json'
import type firewall from '../locales/en/firewall.json'
import type storageBuckets from '../locales/en/storageBuckets.json'
import type rules from '../locales/en/rules.json'
import type deploys from '../locales/en/deploys.json'
import type settings from '../locales/en/settings.json'
import type dashboard from '../locales/en/dashboard.json'
import type auditLog from '../locales/en/auditLog.json'
import type streams from '../locales/en/streams.json'
import type networkProxy from '../locales/en/networkProxy.json'
import type privateNetworks from '../locales/en/privateNetworks.json'
import type fixit from '../locales/en/fixit.json'
import type network from '../locales/en/network.json'
import type databases from '../locales/en/databases.json'
import type https from '../locales/en/https.json'
import type nodes from '../locales/en/nodes.json'
import type access from '../locales/en/access.json'
import type logPipeline from '../locales/en/logPipeline.json'
import type logBudgets from '../locales/en/logBudgets.json'
import type environments from '../locales/en/environments.json'
import type exposure from '../locales/en/exposure.json'
import type appExposure from '../locales/en/appExposure.json'
import type appAccess from '../locales/en/appAccess.json'
import type migration from '../locales/en/migration.json'
import type domains from '../locales/en/domains.json'
import type attention from '../locales/en/attention.json'
import type setup from '../locales/en/setup.json'
import type updates from '../locales/en/updates.json'
import type infraCanvas from '../locales/en/infraCanvas.json'
import type databaseAccess from '../locales/en/databaseAccess.json'
import type databaseUpgrades from '../locales/en/databaseUpgrades.json'
import type databaseVersions from '../locales/en/databaseVersions.json'
import type iam from '../locales/en/iam.json'
import type signIn from '../locales/en/signIn.json'
import type security from '../locales/en/security.json'
import type traffic from '../locales/en/traffic.json'
import type dns from '../locales/en/dns.json'
import type domainPolicies from '../locales/en/domainPolicies.json'
import type observability from '../locales/en/observability.json'
import type trialApp from '../locales/en/trialApp.json'
import type previews from '../locales/en/previews.json'
import type backups from '../locales/en/backups.json'
import type sleepHistory from '../locales/en/sleepHistory.json'
import type gitBuild from '../locales/en/gitBuild.json'
import type teams from '../locales/en/teams.json'
import type compose from '../locales/en/compose.json'
import type smartDeploy from '../locales/en/smartDeploy.json'
import type dataExport from '../locales/en/dataExport.json'
import type eventWebhooks from '../locales/en/eventWebhooks.json'
import type maintenance from '../locales/en/maintenance.json'
import type buildPerformance from '../locales/en/buildPerformance.json'
import type deployPolicies from '../locales/en/deployPolicies.json'
import type compliance from '../locales/en/compliance.json'
import type deploySafety from '../locales/en/deploySafety.json'
import type jobs from '../locales/en/jobs.json'
import type pipelines from '../locales/en/pipelines.json'
import type budget from '../locales/en/budget.json'
import type autoscale from '../locales/en/autoscale.json'
import type envEditor from '../locales/en/envEditor.json'
import type resourceAdvisor from '../locales/en/resourceAdvisor.json'
import type plugins from '../locales/en/plugins.json'
import type help from '../locales/en/help.json'
import type traces from '../locales/en/traces.json'
import type errors from '../locales/en/errors.json'
import type edgeAccess from '../locales/en/edgeAccess.json'
import type trafficDetail from '../locales/en/trafficDetail.json'
import type betaFeatures from '../locales/en/betaFeatures.json'
import type newApp from '../locales/en/newApp.json'
import type getStarted from '../locales/en/getStarted.json'
import type usage from '../locales/en/usage.json'
import type resources from '../locales/en/resources.json'
import type deployDetail from '../locales/en/deployDetail.json'
import type deployTimeline from '../locales/en/deployTimeline.json'
import type images from '../locales/en/images.json'
import type buildCenter from '../locales/en/buildCenter.json'
import type functions from '../locales/en/functions.json'
import type secretRotation from '../locales/en/secretRotation.json'
import type templates from '../locales/en/templates.json'
import type models from '../locales/en/models.json'
import type aiAssistant from '../locales/en/aiAssistant.json'
import type demo from '../locales/en/demo.json'
import type secretScan from '../locales/en/secretScan.json'
import type sso from '../locales/en/sso.json'
import type cdn from '../locales/en/cdn.json'

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
      privateNetworks: typeof privateNetworks
      fixit: typeof fixit
      network: typeof network
      databases: typeof databases
      https: typeof https
      nodes: typeof nodes
      access: typeof access
      logPipeline: typeof logPipeline
      logBudgets: typeof logBudgets
      environments: typeof environments
      exposure: typeof exposure
      appExposure: typeof appExposure
      appAccess: typeof appAccess
      migration: typeof migration
      domains: typeof domains
      attention: typeof attention
      setup: typeof setup
      updates: typeof updates
      infraCanvas: typeof infraCanvas
      databaseAccess: typeof databaseAccess
      databaseUpgrades: typeof databaseUpgrades
      databaseVersions: typeof databaseVersions
      iam: typeof iam
      signIn: typeof signIn
      security: typeof security
      traffic: typeof traffic
      dns: typeof dns
      domainPolicies: typeof domainPolicies
      observability: typeof observability
      previews: typeof previews
      backups: typeof backups
      sleepHistory: typeof sleepHistory
      gitBuild: typeof gitBuild
      teams: typeof teams
      compose: typeof compose
      smartDeploy: typeof smartDeploy
      dataExport: typeof dataExport
      eventWebhooks: typeof eventWebhooks
      compliance: typeof compliance
      deploySafety: typeof deploySafety
      jobs: typeof jobs
      pipelines: typeof pipelines
      budget: typeof budget
      autoscale: typeof autoscale
      envEditor: typeof envEditor
      resourceAdvisor: typeof resourceAdvisor
      plugins: typeof plugins
      help: typeof help
      traces: typeof traces
      errors: typeof errors
      edgeAccess: typeof edgeAccess
      trafficDetail: typeof trafficDetail
      betaFeatures: typeof betaFeatures
      newApp: typeof newApp
      getStarted: typeof getStarted
      usage: typeof usage
      resources: typeof resources
      deployTimeline: typeof deployTimeline
      images: typeof images
      buildCenter: typeof buildCenter
      functions: typeof functions
      secretRotation: typeof secretRotation
      models: typeof models
      templates: typeof templates
      aiAssistant: typeof aiAssistant
      demo: typeof demo
      secretScan: typeof secretScan
      maintenance: typeof maintenance
      buildPerformance: typeof buildPerformance
      sso: typeof sso
      cdn: typeof cdn
      trialApp: typeof trialApp
      deployPolicies: typeof deployPolicies
      deployDetail: typeof deployDetail
      firewall: typeof firewall
      rules: typeof rules
      storageBuckets: typeof storageBuckets
    }
  }
}
