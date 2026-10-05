import type { LandingCard } from './landingTypes'

export const proofStats = [
  { value: '0.7%', label: 'CPU at idle, production-test VPS' },
  { value: '146 MB', label: 'RAM at idle, same server' },
  { value: '500', label: 'apps in the idle benchmark' },
  { value: '91 MB', label: 'memory at 500 apps, development build' },
]

export const proofLink = { text: 'See the measurements and what is not proven yet', href: '/case-studies' }

export const pathCards: LandingCard[] = [
  {
    title: 'Try it in 5 minutes',
    icon: 'terminal',
    body: 'Run the install script on a Linux box, finish the setup wizard, and deploy your first app.',
    link: { text: 'Open the quickstart', href: '/getting-started' },
  },
  {
    title: 'See a demo',
    icon: 'eye',
    body: 'Click through the dashboard with sample data before you install anything.',
    link: { text: 'Open the demo', href: '/demo' },
  },
  {
    title: 'Switching from another platform?',
    icon: 'branch',
    body: 'See how Levelrail differs from Coolify and what moving over involves.',
    link: { text: 'Read the Coolify guide', href: '/coolify-alternative' },
  },
]

export const compare = {
  heading: 'How it compares',
  left: 'SSH + shell out',
  right: 'Levelrail',
  rows: [
    {
      label: 'Server management',
      left: 'SSHes into every node and runs docker CLI commands, then parses the text output.',
      right: 'The agent dials out over mTLS and talks to the Docker Engine API directly. Nothing shells out to the docker CLI.',
    },
    {
      label: 'Orchestration',
      left: 'Polling loops that often leave no recorded reason for why a resource is in its current state.',
      right: 'A level-triggered reconciler diffs desired against observed state and writes a status condition with a reason after every pass.',
    },
    {
      label: 'Observability',
      left: 'Often an add-on: install Grafana or Loki yourself, then wire them up for metrics and logs.',
      right: 'Node-local metrics at 15 second resolution and full-text log search are built in, no separate install.',
    },
    {
      label: 'Footprint',
      left: 'Often a stack of separate services: reverse proxy, metrics store, log store, dashboard.',
      right: 'SQLite in WAL mode, an embedded Caddy ingress, and an embedded dashboard: one control plane binary on one node.',
    },
  ],
  more: { text: 'See the full comparison against Coolify, Dokploy, CapRover, Dokku, and Kamal', href: '/comparison' },
}

export const switching = [
  { text: 'Coolify', link: '/coolify-alternative' },
  { text: 'Dokploy', link: '/dokploy-alternative' },
  { text: 'Vercel', link: '/vercel-alternative' },
  { text: 'Heroku', link: '/heroku-alternative' },
  { text: 'Railway', link: '/railway-alternative' },
  { text: 'Demo', link: '/demo' },
  { text: 'Pricing', link: '/pricing' },
]

export const gallery = [
  { src: '/assets/screenshots/apps-list.png', url: 'levelrail.local/apps', alt: 'Levelrail apps list showing all services across nodes at a glance', caption: 'Every app across your nodes at a glance' },
  { src: '/assets/screenshots/deploy-history.png', url: 'levelrail.local/apps/web/deploys', alt: 'Levelrail deploy history view with one-click rollback', caption: 'Deploy history with one-click rollback' },
  { src: '/assets/screenshots/logs.png', url: 'levelrail.local/apps/web/logs', alt: 'Levelrail live log viewer with full-text search', caption: 'Live logs with full-text search' },
  { src: '/assets/screenshots/nodes.png', url: 'levelrail.local/nodes', alt: 'Levelrail nodes list showing node health and placement', caption: 'Node health and placement' },
]
