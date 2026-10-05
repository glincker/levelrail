export type NavIconKey =
  | 'demo'
  | 'caseStudies'
  | 'architecture'
  | 'security'
  | 'roadmap'
  | 'featureStatus'
  | 'gettingStarted'
  | 'installing'
  | 'deploying'
  | 'multiNode'
  | 'cli'
  | 'api'
  | 'appSpec'
  | 'troubleshooting'
  | 'comparison'
  | 'alternative'
  | 'changelog'
  | 'privacy'
  | 'github'
  | 'discussions'
  | 'discord'

export interface NavItem {
  text: string
  link: string
  description: string
  icon: NavIconKey
  external?: boolean
}

export interface NavGroup {
  label: string
  items: NavItem[]
}

export interface NavPlainLink {
  text: string
  link: string
}

export type NavEntry =
  | { kind: 'group'; group: NavGroup }
  | { kind: 'link'; link: NavPlainLink }

const REPO = 'https://github.com/glincker/levelrail'

export const navEntries: NavEntry[] = [
  {
    kind: 'group',
    group: {
      label: 'Product',
      items: [
        { text: 'Demo', link: '/demo', icon: 'demo', description: 'See a deploy from install to rollback' },
        { text: 'Case studies', link: '/case-studies', icon: 'caseStudies', description: 'Measured results, stated honestly' },
        { text: 'Architecture', link: '/architecture', icon: 'architecture', description: 'Reconciler, agent, builds and ingress' },
        { text: 'Security overview', link: '/security', icon: 'security', description: 'Secrets, sessions, TLS and access control' },
        { text: 'Roadmap', link: '/roadmap', icon: 'roadmap', description: 'What is shipped, in progress and planned' },
        { text: 'Feature status', link: '/feature-status', icon: 'featureStatus', description: 'Maturity and evidence for each feature' },
        { text: 'Zero-downtime deploys', link: '/zero-downtime-deploys', icon: 'deploying', description: 'Readiness-gated cutover and rollback' },
        { text: 'Preview environments', link: '/preview-environments', icon: 'deploying', description: 'A running copy for every pull request' },
      ],
    },
  },
  {
    kind: 'group',
    group: {
      label: 'Docs',
      items: [
        { text: 'Getting started', link: '/getting-started', icon: 'gettingStarted', description: 'Install and deploy your first app' },
        { text: 'Installing', link: '/installing', icon: 'installing', description: 'Requirements and every install method' },
        { text: 'Deploying apps', link: '/deploying-apps', icon: 'deploying', description: 'Deploy, roll back and operate apps' },
        { text: 'Multi-node', link: '/multi-node', icon: 'multiNode', description: 'Add nodes, placement and the mesh' },
        { text: 'CLI reference', link: '/cli-reference', icon: 'cli', description: 'Every command, grouped, with examples' },
        { text: 'API reference', link: '/api-reference', icon: 'api', description: 'The control plane HTTP API' },
        { text: 'App spec reference', link: '/app-spec-reference', icon: 'appSpec', description: 'Every field of app.yaml' },
        { text: 'Troubleshooting', link: '/troubleshooting', icon: 'troubleshooting', description: 'Fixes for common problems' },
      ],
    },
  },
  {
    kind: 'group',
    group: {
      label: 'Compare',
      items: [
        { text: 'Full comparison', link: '/comparison', icon: 'comparison', description: 'Coolify, Dokploy, CapRover, Dokku, Kamal' },
        { text: 'Coolify alternative', link: '/coolify-alternative', icon: 'alternative', description: 'A lighter way to self-host' },
        { text: 'Dokploy alternative', link: '/dokploy-alternative', icon: 'alternative', description: 'Agent-based, no Swarm' },
        { text: 'Vercel alternative', link: '/vercel-alternative', icon: 'alternative', description: 'Run your own deploy platform' },
        { text: 'Heroku alternative', link: '/heroku-alternative', icon: 'alternative', description: 'Git push deploys on your servers' },
        { text: 'Railway alternative', link: '/railway-alternative', icon: 'alternative', description: 'Self-hosted, on your own hardware' },
        { text: 'CapRover alternative', link: '/caprover-alternative', icon: 'alternative', description: 'Plain Docker, no Swarm' },
        { text: 'Dokku alternative', link: '/dokku-alternative', icon: 'alternative', description: 'A dashboard and more servers' },
        { text: 'Kamal alternative', link: '/kamal-alternative', icon: 'alternative', description: 'Zero downtime plus a control plane' },
      ],
    },
  },
  {
    kind: 'group',
    group: {
      label: 'Resources',
      items: [
        { text: 'Changelog', link: '/changelog/', icon: 'changelog', description: 'Release notes for every version' },
        { text: 'Developers', link: '/developers', icon: 'api', description: 'API, CLI and MCP for builders' },
        { text: 'Contribute', link: '/contribute', icon: 'discussions', description: 'Ways to help build Levelrail' },
        { text: 'Privacy', link: '/privacy', icon: 'privacy', description: 'What stays on your servers' },
        { text: 'GitHub', link: REPO, icon: 'github', description: 'Source code and issues', external: true },
        { text: 'Discussions', link: `${REPO}/discussions`, icon: 'discussions', description: 'Questions and ideas', external: true },
        { text: 'Discord', link: 'https://discord.gg/Ar5pcaZB99', icon: 'discord', description: 'Chat with the community', external: true },
      ],
    },
  },
  { kind: 'link', link: { text: 'Pricing', link: '/pricing' } },
]

export function flattenNav(): NavPlainLink[] {
  return navEntries.flatMap((entry) =>
    entry.kind === 'link'
      ? [entry.link]
      : entry.group.items.map(({ text, link }) => ({ text, link })),
  )
}
