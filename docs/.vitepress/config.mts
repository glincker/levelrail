import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'
import { buildEnd, changelogHead, changelogPageData } from './changelog.mts'
import { buildLlmsFullTxt } from './llmsFull.mts'
import { writeRawMarkdown } from './rawMarkdown.mts'
import { faqItems } from './theme/faqData'

const description =
  'A self-hosted deployment platform whose agent talks to Docker’s own Engine API directly, ' +
  'no SSH or CLI shelling, with metrics and log storage built into the core.'

const siteUrl = 'https://levelrail.com'

// public/favicon.svg is served byte-for-byte at a fixed path (no content
// hash), behind GitHub Pages' CDN on top of the browser's own cache, so a
// redeploy alone doesn't guarantee a visitor sees the new file. Bump this
// whenever favicon.svg's actual content changes; it's appended everywhere
// the file is referenced below.
const faviconVersion = 2

// Defined once and reused for both the sidebar itself and
// pageToSection below (canonicalUrl/BreadcrumbList in transformHead),
// so the two never drift out of sync.
const sidebarGroups = [
  {
    text: 'Tutorials',
    items: [
      { text: 'Getting started', link: '/getting-started' },
      { text: 'All tutorials', link: '/tutorials/' },
      { text: 'Deploy a Docker app', link: '/tutorials/deploy-a-docker-app' },
      { text: 'Zero-downtime deploys', link: '/tutorials/zero-downtime-deploys-with-health-checks' },
      { text: 'Deploy from GitHub Actions', link: '/tutorials/deploy-from-github-actions' },
      { text: 'Connect an app to Postgres', link: '/tutorials/connect-an-app-to-postgres' },
      { text: 'Back up Postgres to S3', link: '/tutorials/back-up-postgres-to-s3' },
      { text: 'Self-host Vaultwarden', link: '/tutorials/self-host-vaultwarden' },
      { text: 'Debug with logs and metrics', link: '/tutorials/debug-an-app-with-logs-and-metrics' },
    ],
  },
  {
    text: 'How-to guides',
    collapsed: true,
    items: [
      { text: 'Installing', link: '/installing' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'Email notifications', link: '/email-notifications' },
      {
        text: 'Deploying',
        collapsed: true,
        items: [
          { text: 'Deploying apps', link: '/deploying-apps' },
          { text: 'Deploy safety', link: '/deploy-safety' },
          { text: 'Deploy failures', link: '/deploy-failures' },
          { text: 'Scheduled deploys', link: '/scheduled-deploys' },
          { text: 'Deployments page', link: '/deployments-page' },
          { text: 'Deploy status badge', link: '/deploy-status-badge' },
          { text: 'Screenshots', link: '/screenshots' },
          { text: 'Docker', link: '/docker' },
        ],
      },
      {
        text: 'Networking and multi-node',
        collapsed: true,
        items: [
          { text: 'Domains and ingress', link: '/domains-and-ingress' },
          { text: 'Load balancing', link: '/load-balancing' },
          { text: 'ACME verification runbook', link: '/acme-verification-runbook' },
          { text: 'Multi-node', link: '/multi-node' },
          { text: 'Multi-node quickstart', link: '/multi-node-quickstart' },
          { text: 'Node provisioning', link: '/node-provisioning' },
          {
            text: 'Multi-cloud provisioning quickstart',
            link: '/multi-cloud-provisioning',
          },
          { text: 'Network topology', link: '/network-topology' },
        ],
      },
      {
        text: 'Databases and storage',
        collapsed: true,
        items: [
          { text: 'Managing databases', link: '/managing-databases' },
          {
            text: 'Connecting apps to databases',
            link: '/connecting-apps-to-databases',
          },
          { text: 'Backups and storage', link: '/backups-and-storage' },
          { text: 'Object storage', link: '/object-storage' },
          { text: 'Log archive', link: '/log-archive' },
          { text: 'Control plane backup', link: '/control-plane-backup' },
          { text: 'Disaster recovery', link: '/disaster-recovery' },
        ],
      },
      {
        text: 'CI/CD and automation',
        collapsed: true,
        items: [
          { text: 'Deploying from GitHub Actions', link: '/github-actions' },
          { text: 'Pipelines', link: '/pipelines' },
          { text: 'Platform as code', link: '/platform-as-code' },
          { text: 'Git integrations', link: '/git-integrations' },
        ],
      },
      {
        text: 'Security and access',
        collapsed: true,
        items: [
          { text: 'Feature flags', link: '/feature-flags' },
          { text: 'Master key rotation', link: '/master-key-rotation' },
          { text: 'Identity and access', link: '/identity-and-access' },
          {
            text: 'Projects and organizations',
            link: '/projects-and-organizations',
          },
        ],
      },
      {
        text: 'Scale and operations',
        collapsed: true,
        items: [
          { text: 'Managing apps at scale', link: '/managing-apps-at-scale' },
          { text: 'Resilience', link: '/resilience' },
          { text: 'Observability', link: '/observability' },
          { text: 'Public status page', link: '/status-page' },
          { text: 'AI models', link: '/ai-models' },
        ],
      },
      {
        text: 'Templates and catalog',
        collapsed: true,
        items: [
          { text: 'Templates and registry', link: '/templates-and-registry' },
          { text: 'Template catalog', link: '/template-catalog' },
          { text: 'Starter kit templates', link: '/templates' },
        ],
      },
      {
        text: 'Migrating from other platforms',
        collapsed: true,
        items: [
          {
            text: 'Migrating from Coolify, Dokploy, or CapRover',
            link: '/migrating-from-coolify-dokploy-and-caprover',
          },
          { text: 'Migrating from Vercel', link: '/migrating-from-vercel' },
        ],
      },
    ],
  },
  {
    text: 'Reference',
    collapsed: true,
    items: [
      { text: 'App spec reference', link: '/app-spec-reference' },
      { text: 'Feature catalog', link: '/feature-catalog' },
      { text: 'CLI reference', link: '/cli-reference' },
      { text: 'API reference', link: '/api-reference' },
      { text: 'API explorer', link: '/api-explorer' },
      { text: 'MCP tool surface', link: '/mcp-tool-surface' },
    ],
  },
  {
    text: 'Explanation',
    collapsed: true,
    items: [
      { text: 'Architecture', link: '/architecture' },
      { text: 'Security overview', link: '/security' },
      { text: 'Threat model', link: '/threat-model' },
      { text: 'Security alert verdicts', link: '/security-alert-verdicts' },
      { text: 'Comparison', link: '/comparison' },
      { text: 'Who Levelrail is for', link: '/use-cases' },
    ],
  },
  {
    text: 'Design proposals',
    collapsed: true,
    items: [
      {
        text: 'Git provider integrations',
        link: '/design/git-provider-integrations',
      },
    ],
  },
  {
    text: 'Status',
    items: [
      { text: 'Roadmap', link: '/roadmap' },
      { text: 'Feature status', link: '/feature-status' },
      { text: 'Experimental features', link: '/experimental-features' },
      { text: 'Performance', link: '/performance' },
      { text: 'CI', link: '/ci' },
      { text: 'Changelog', link: '/changelog/' },
    ],
  },
  {
    text: 'Docs index',
    collapsed: true,
    items: [{ text: 'Overview', link: '/README' }],
  },
]

// Maps a sidebar link's path (no leading slash) to its group's title,
// for transformHead's BreadcrumbList below. Built from sidebarGroups
// itself so the breadcrumb's middle segment can never list a section a
// page doesn't actually appear under in the real sidebar.
// pageDepth tracks nesting (1 = a group's direct item, 2+ = inside a
// collapsed subgroup), reused by the sitemap's priority below.
const pageToSection = new Map<string, string>()
const pageDepth = new Map<string, number>()
function collectPages(
  items: typeof sidebarGroups[number]['items'],
  sectionText: string,
  depth = 1,
) {
  for (const item of items) {
    if ('link' in item) {
      pageToSection.set(item.link.replace(/^\//, ''), sectionText)
      pageDepth.set(item.link.replace(/^\//, ''), depth)
    }
    if ('items' in item) {
      collectPages(item.items, sectionText, depth + 1)
    }
  }
}
for (const group of sidebarGroups) {
  collectPages(group.items, group.text)
}

// Base sitemap priority per top-level sidebar section; a page one or
// more collapsed subgroups deep gets a small penalty on top of this.
const sectionPriority: Record<string, number> = {
  Tutorials: 0.9,
  'How-to guides': 0.7,
  Reference: 0.6,
  Explanation: 0.6,
  Status: 0.4,
  'Design proposals': 0.3,
  'Docs index': 0.3,
}
function sitemapPriority(url: string): number {
  if (url === '') return 1.0
  if (url === 'getting-started') return 0.9
  if (url.startsWith('changelog/') && url !== 'changelog/') return 0.3
  const section = pageToSection.get(url)
  if (!section) return 0.4
  const base = sectionPriority[section] ?? 0.5
  const depth = pageDepth.get(url) ?? 1
  return depth > 1 ? Math.max(0.3, base - 0.1) : base
}

export default withMermaid({
  title: 'Levelrail',
  description,

  // lastUpdated also drives the sitemap's lastmod below (VitePress reads
  // each page's own last git-commit date once this is on).
  lastUpdated: true,

  // TODO: revisit once glinr.com/levelrail (a path, not this subdomain)
  // becomes possible, per the root CLAUDE.md's stated long-term target.
  sitemap: {
    hostname: siteUrl,
    transformItems: (items) =>
      items.map((item) => ({ ...item, priority: sitemapPriority(item.url) })),
  },

  head: [
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'theme-color', content: '#0b0e14' }],
    ['link', { rel: 'icon', href: `/favicon.svg?v=${faviconVersion}`, type: 'image/svg+xml' }],
    [
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'SoftwareApplication',
        name: 'Levelrail',
        description,
        applicationCategory: 'DeveloperApplication',
        operatingSystem: 'Linux',
        offers: {
          '@type': 'Offer',
          price: '0',
          priceCurrency: 'USD',
        },
        license: 'https://www.apache.org/licenses/LICENSE-2.0',
        url: `${siteUrl}/`,
        codeRepository: 'https://github.com/glincker/levelrail',
        publisher: {
          '@type': 'Organization',
          name: 'GLINCKER',
          url: 'https://glincker.com',
        },
      }),
    ],
  ],

  cleanUrls: true,
  appearance: 'dark',

  // mermaid's transitive fastdom dep is plain CJS with no default export;
  // esbuild's auto-interop misses it, so force it explicitly.
  vite: {
    optimizeDeps: {
      include: ['mermaid > fastdom', 'mermaid > fastdom/extensions/fastdom-promised.js'],
      needsInterop: ['mermaid > fastdom', 'mermaid > fastdom/extensions/fastdom-promised.js'],
    },
  },

  // A handful of docs link up to files outside docs/ (root README.md,
  // CHANGELOG.md, /adr) that exist in the repo but sit outside this
  // site's srcDir, by design: docs/README.md documents that these pages
  // are meant to render correctly on GitHub too, not just in this site.
  ignoreDeadLinks: [/\.\.\//],

  themeConfig: {
    logo: `/favicon.svg?v=${faviconVersion}`,

    nav: [
      { text: 'Guide', link: '/getting-started' },
      { text: 'Reference', link: '/app-spec-reference' },
      { text: 'Compare', link: '/comparison' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'Roadmap', link: '/roadmap' },
      { text: 'Changelog', link: '/changelog/' },
    ],

    sidebar: sidebarGroups,

    socialLinks: [
      { icon: 'github', link: 'https://github.com/glincker/levelrail' },
    ],

    editLink: {
      pattern: 'https://github.com/glincker/levelrail/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },

    search: {
      provider: 'local',
    },

    footer: {
      message: 'Released under the Apache 2.0 License.',
      copyright: 'Copyright © GLINCKER',
    },

    lastUpdated: {
      formatOptions: { dateStyle: 'medium' },
    },

    // Read by PageActions.vue so the "open in <AI tool>" links and the
    // raw-markdown fetch don't hardcode the domain a second time.
    siteUrl,
  },

  // Per-page canonical link and BreadcrumbList: VitePress doesn't add
  // either by default. Skips index.md (the home layout has no
  // meaningful breadcrumb) and any page pageToSection doesn't
  // recognize (docs/README.md, ADRs reached via ../adr, etc.).
  transformPageData(pageData) {
    return changelogPageData(pageData)
  },

  buildEnd: async (config) => {
    await buildEnd(config, siteUrl)
    buildLlmsFullTxt(config, siteUrl)
    writeRawMarkdown(config)
  },

  transformHead({ pageData }) {
    const path = pageData.relativePath.replace(/\.md$/, '').replace(/(^|\/)index$/, '$1')
    const canonicalUrl = `${siteUrl}/${path}`
    const title = pageData.frontmatter.title || pageData.title || 'Levelrail'
    const pageDescription = pageData.frontmatter.description || pageData.description || description
    // Homepage keeps its own product screenshot; every other page gets one
    // shared generic docs card rather than all pages sharing the homepage's.
    const ogImage = `${siteUrl}/assets/${path === '' ? 'screenshots/app-overview.png' : 'og-docs.png'}`
    const head: [string, Record<string, string>, string?][] = [
      ['link', { rel: 'canonical', href: canonicalUrl }],
      ['meta', { property: 'og:type', content: pageData.params?.tag ? 'article' : 'website' }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: pageDescription }],
      ['meta', { property: 'og:url', content: canonicalUrl }],
      ['meta', { property: 'og:image', content: ogImage }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: pageDescription }],
      ['meta', { name: 'twitter:image', content: ogImage }],
      ...changelogHead(pageData, siteUrl),
    ]

    // FAQPage structured data for Google's FAQ rich-result eligibility.
    // path === '' is the homepage (index.md), the only page with
    // <FaqSection />. faqItems is shared with FaqSection.vue itself so
    // this can never drift from what's actually rendered.
    if (path === '') {
      head.push([
        'script',
        { type: 'application/ld+json' },
        JSON.stringify({
          '@context': 'https://schema.org',
          '@type': 'FAQPage',
          mainEntity: faqItems.map((item) => ({
            '@type': 'Question',
            name: item.q,
            acceptedAnswer: { '@type': 'Answer', text: item.a },
          })),
        }),
      ])
    }

    const section = pageToSection.get(path)
    if (section) {
      head.push([
        'script',
        { type: 'application/ld+json' },
        JSON.stringify({
          '@context': 'https://schema.org',
          '@type': 'BreadcrumbList',
          itemListElement: [
            { '@type': 'ListItem', position: 1, name: 'Levelrail', item: `${siteUrl}/` },
            { '@type': 'ListItem', position: 2, name: section, item: canonicalUrl },
            { '@type': 'ListItem', position: 3, name: pageData.title, item: canonicalUrl },
          ],
        }),
      ])
    }

    return head
  },

  mermaid: {
    theme: 'base',
    themeVariables: {
      primaryColor: '#161b24',
      primaryTextColor: '#e4e4e7',
      primaryBorderColor: '#2fb3dc',
      lineColor: '#a1a1aa',
      secondaryColor: '#10141c',
      tertiaryColor: '#0b0e14',
      fontSize: '16px',
    },
    flowchart: {
      useMaxWidth: false,
      padding: 16,
    },
  },
})
