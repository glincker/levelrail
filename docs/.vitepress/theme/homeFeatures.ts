import type { LandingCard } from './landingTypes'

export const homeFeatures: LandingCard[] = [
  {
    title: 'Zero-downtime deploys',
    icon: 'history',
    body: 'Blue-green (the default) and rolling deploys move traffic only after the new container passes its readiness probe. Prior images stay pinned, so rollback is always one click away.',
    visual: [
      { k: 'out', t: 'strategy: blue-green | rolling | recreate' },
      { k: 'out', t: 'new container: readiness probe passed' },
      { k: 'ok', t: 'traffic moved, previous image pinned' },
    ],
  },
  {
    title: 'Observability built in',
    icon: 'chart',
    body: 'Node-local metrics at 15s resolution and full-text log search, with 15 days of retention by default and no separate Grafana or Loki install. Deploy markers overlay on metric charts.',
    routes: [
      { method: 'GET', path: '/api/v1/apps/{name}/logs' },
      { method: 'GET', path: '/api/v1/apps/{name}/logs/stream' },
      { method: 'GET', path: '/api/v1/apps-metrics' },
    ],
  },
  {
    title: 'Eight managed database engines',
    icon: 'database',
    body: 'Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse, with scheduled backups, restore, and automatic post-backup verification.',
    visual: [
      { k: 'out', t: 'postgres   redis      mysql      mongodb' },
      { k: 'out', t: 'mariadb    keydb      dragonfly  clickhouse' },
      { k: 'ok', t: 'backups: scheduled, restore, verified' },
    ],
  },
  {
    title: 'Multi-node, no inbound ports',
    icon: 'network',
    body: 'Add servers with a one-time join token. Every agent dials out over mTLS, so managed servers open no inbound ports, and you can cordon, drain, and pin apps to nodes. The WireGuard mesh is beta.',
    visual: [
      { k: 'out', t: 'agent -> control plane (outbound, mTLS)' },
      { k: 'out', t: 'inbound ports opened on the node: none' },
      { k: 'ok', t: 'cordon, drain, pin apps to nodes' },
    ],
  },
  {
    title: 'Know what needs attention',
    icon: 'bell',
    body: 'A Status page and the attention CLI command list failing apps, offline nodes, expiring certificates, and doctor findings. Alerts reach you through 18 notification channel kinds, including Slack, Discord, email, Telegram, PagerDuty, and ntfy.',
    visual: [
      { k: 'cmd', t: 'levelrail-cli attention' },
      { k: 'out', t: 'app failing | cert expiring | node offline' },
    ],
  },
  {
    title: 'Resource-scoped IAM',
    icon: 'key',
    body: 'AWS-IAM-shaped Allow/Deny policies scoped to a specific app or database, with a full audit log and CSV export, in the free Apache 2.0 core.',
    visual: [
      { k: 'out', t: 'allow: app:web:deploy' },
      { k: 'out', t: 'deny:  database:main:*' },
      { k: 'ok', t: 'audit log with CSV export' },
    ],
  },
  {
    title: '311 one-click templates',
    icon: 'cube',
    body: 'Self-hosted services such as n8n, Gitea, Uptime Kuma, and Vaultwarden, each a Compose file that Levelrail deploys and manages like any other app.',
    routes: [{ method: 'GET', path: '/api/v1/templates' }],
  },
  {
    title: 'AI-ready API',
    icon: 'robot',
    body: '156 MCP tools (beta) backed by the same HTTP API the dashboard runs on, so AI tools can list apps, read logs, and diagnose a failed deploy directly.',
    routes: [
      { method: 'GET', path: '/api/v1/apps' },
      { method: 'GET', path: '/api/v1/apps/{name}/logs' },
      { method: 'POST', path: '/api/v1/apps/{name}/deploy' },
    ],
    link: { text: 'MCP tool surface', href: '/mcp-tool-surface' },
  },
]
