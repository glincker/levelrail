const APP_SLUG_DOCS: Record<string, string> = {
  deploys: 'deploying-apps',
  source: 'git-integrations',
  'deploy-settings': 'deploy-safety',
  pipelines: 'pipelines',
  domains: 'domains-and-ingress',
  loadbalancer: 'load-balancing',
  network: 'domains-and-ingress',
  metrics: 'observability',
  logs: 'observability',
  alerts: 'observability',
  integrations: 'integrations',
  'feature-flags': 'feature-flags',
  environment: 'deploying-apps',
  services: 'deploying-apps',
}

const PREFIX_DOCS: [string, string][] = [
  ['/databases', 'managing-databases'],
  ['/backups', 'backups-and-storage'],
  ['/loadbalancers', 'load-balancing'],
  ['/domains', 'domains-and-ingress'],
  ['/nodes', 'multi-node'],
  ['/models', 'ai-models'],
  ['/ai-assistant', 'ai-assistant'],
  ['/approvals', 'deploy-safety'],
  ['/pipelines', 'pipelines'],
  ['/projects', 'projects-and-organizations'],
  ['/status', 'troubleshooting'],
  ['/settings', 'identity-and-access'],
]

export function helpDocForPath(pathname: string): string | undefined {
  const app = /^\/apps\/[^/]+\/([^/]+)/.exec(pathname)?.[1]
  if (app) return APP_SLUG_DOCS[app]
  return PREFIX_DOCS.find(
    ([p]) => pathname === p || pathname.startsWith(`${p}/`),
  )?.[1]
}
