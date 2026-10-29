// Keyword and synonym data for the command palette. Pure data: ranking lives
// in paletteSearch.ts. Keys are the palette item keys from ROUTE_ENTRIES.

/** Extra match words per item key, ranked below the label. */
export const ITEM_KEYWORDS: Readonly<Record<string, string>> = {
  'nav-databases':
    'db database postgres postgresql mysql mariadb redis mongo sql data',
  'action-create-database': 'db database postgres mysql redis new provision',
  'nav-backups': 'backup restore snapshot db database dump recovery',
  'nav-assets': 'assets cdn images bucket r2 s3 upload files media static',
  'settings-backup-targets': 'backup s3 bucket storage restore destination',
  'settings-storage': 's3 bucket object storage volume disk',
  'nav-domains': 'domain dns ssl tls cert certificate https hostname url',
  'nav-dns': 'dns domain nameserver record zone cname',
  'nav-firewall': 'firewall ports security network ingress rules',
  'nav-private-networks': 'network vpc subnet wireguard mesh private',
  'action-private-networks': 'network vpc subnet new private',
  'nav-loadbalancers': 'load balancer lb traffic proxy ingress',
  'settings-registry-credentials':
    'container registry docker image images ghcr hub pull credentials runtime',
  'nav-builds':
    'container image images docker build buildpack registry runtime',
  'nav-functions': 'serverless lambda function container runtime',
  'nav-deployments':
    'deploy deploys release rollout progress rollback logs history predeploy snapshot',
  'settings-dr-report': 'recovery disaster dr report restore rto rpo',
  'settings-heartbeat': 'heartbeat dead man ping uptime monitor notify',
  'settings-deploy-policies': 'ci gate deploy policy checks required status',
  'settings-cloudflare-tunnel':
    'tunnel cloudflare cloudflared expose hostname no ports',
  'nav-deploy-timeline': 'deploy deploys history releases timeline events',
  'nav-approvals': 'approve review gate deploy approvals',
  'nav-usage': 'cost billing spend money usage price',
  'nav-costs': 'cost billing spend money price budget',
  'action-nodes': 'server host machine vps node',
  'nav-projects': 'project group workspace environment',
  'nav-pipelines': 'pipeline ci workflow stages promote',
  'nav-models': 'ai llm model gpu',
  'nav-assistant': 'ai chat llm help assistant',
  'nav-console': 'terminal shell cli ssh exec',
  'nav-help': 'docs documentation support guide',
  'settings-stacks': 'stack gitops iac revision rollback drift plan prune',
  'settings-stack-templates': 'stack template bundle starter iac gitops',
  'settings-tokens': 'api token key secret pat access',
  'settings-security': 'secret secrets password 2fa mfa auth session',
  'settings-oauth': 'sso login auth oidc github google sign in',
  'settings-users': 'user member team people invite',
  'settings-organizations': 'org organization team tenant',
  'settings-email': 'smtp mail email notification',
  'settings-notification-channels':
    'alert slack discord webhook notify notification',
  'settings-git-connections': 'git github gitlab repo repository source',
  'settings-github-app': 'github git repo repository app',
  'settings-agents': 'agent node worker connect',
  'settings-status-page': 'status uptime public page incident',
  'settings-updates': 'update upgrade version release',
  'settings-audit-log': 'audit log history events who did compliance',
  'settings-general': 'general instance name url preferences',
  'settings-account': 'profile password avatar me',
  'action-create-app': 'new app deploy launch service container',
  'action-new-app': 'create app deploy launch service container',
  'action-templates': 'template catalog one click marketplace store apps',
  'action-import': 'migrate migration move platform coolify heroku',
  'action-status': 'health overview uptime',
  'action-alerts': 'alert notification incident page',
  'action-toggle-theme': 'dark light mode appearance color',
}

export interface SynonymGroup {
  /** Typed words (prefix-matched) that trigger the group. */
  terms: readonly string[]
  /** Item keys to suggest, best first. */
  keys: readonly string[]
  /** Template ids or name fragments worth suggesting. */
  templates?: readonly string[]
}

export const SYNONYM_GROUPS: readonly SynonymGroup[] = [
  {
    terms: ['container', 'docker', 'image', 'registry', 'runtime'],
    keys: ['settings-registry-credentials', 'nav-builds', 'nav-functions'],
    templates: ['portainer'],
  },
  {
    terms: ['db', 'database', 'postgres', 'mysql', 'redis', 'sql'],
    keys: ['nav-databases', 'nav-backups', 'action-create-database'],
    templates: ['postgres', 'mysql', 'redis'],
  },
  {
    terms: ['domain', 'dns', 'ssl', 'tls', 'cert', 'certificate'],
    keys: ['nav-domains', 'nav-dns', 'nav-firewall'],
  },
  {
    terms: ['secret', 'env', 'token', 'password'],
    keys: ['settings-security', 'settings-tokens'],
  },
  {
    terms: ['deploy', 'release', 'rollback'],
    keys: ['nav-deployments', 'nav-deploy-timeline', 'nav-approvals'],
  },
  {
    terms: ['backup', 'restore', 'snapshot'],
    keys: ['nav-backups', 'settings-backup-targets', 'nav-databases'],
  },
  {
    terms: ['cost', 'billing', 'spend', 'price'],
    keys: ['nav-usage', 'nav-costs'],
  },
  {
    terms: ['alert', 'notify', 'slack', 'webhook'],
    keys: ['settings-notification-channels', 'action-alerts'],
  },
]

/** Item keys recommended on an empty query, by what the user has so far. */
export const STARTER_KEYS = {
  noApps: ['action-create-app', 'action-import', 'action-templates'],
  hasApps: ['nav-deployments', 'action-apps', 'nav-backups'],
} as const
