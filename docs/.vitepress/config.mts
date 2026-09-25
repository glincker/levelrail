import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'
import { buildEnd, changelogHead, changelogPageData } from './changelog.mts'

const description =
  'A self-hosted deployment platform whose agent talks to Docker’s own Engine API directly, ' +
  'no SSH or CLI shelling, with metrics and log storage built into the core.'

const siteUrl = 'https://levelrail.glinr.com'

// Defined once and reused for both the sidebar itself and
// pageToSection below (canonicalUrl/BreadcrumbList in transformHead),
// so the two never drift out of sync.
const sidebarGroups = [
  {
    text: 'Tutorials',
    items: [{ text: 'Getting started', link: '/getting-started' }],
  },
  {
    text: 'How-to guides',
    collapsed: true,
    items: [
      { text: 'Installing', link: '/installing' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'Docker', link: '/docker' },
      { text: 'Domains and ingress', link: '/domains-and-ingress' },
      { text: 'ACME verification runbook', link: '/acme-verification-runbook' },
      { text: 'Feature flags', link: '/feature-flags' },
      { text: 'Master key rotation', link: '/master-key-rotation' },
      { text: 'Control plane backup', link: '/control-plane-backup' },
      {
        text: 'Migrating from Coolify, Dokploy, or CapRover',
        link: '/migrating-from-coolify-dokploy-and-caprover',
      },
      { text: 'Deploying from GitHub Actions', link: '/github-actions' },
      { text: 'Pipelines', link: '/pipelines' },
      { text: 'Screenshots', link: '/screenshots' },
      { text: 'Deploying apps', link: '/deploying-apps' },
      { text: 'Managing databases', link: '/managing-databases' },
      { text: 'Observability', link: '/observability' },
      { text: 'Multi-node', link: '/multi-node' },
      {
        text: 'Projects and organizations',
        link: '/projects-and-organizations',
      },
      { text: 'Identity and access', link: '/identity-and-access' },
      { text: 'Git integrations', link: '/git-integrations' },
      { text: 'Backups and storage', link: '/backups-and-storage' },
      { text: 'Object storage', link: '/object-storage' },
      { text: 'Templates and registry', link: '/templates-and-registry' },
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
    ],
  },
  {
    text: 'Explanation',
    collapsed: true,
    items: [
      { text: 'Architecture', link: '/architecture' },
      { text: 'Security overview', link: '/security' },
      { text: 'Comparison', link: '/comparison' },
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
      { text: 'Performance', link: '/performance' },
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
const pageToSection = new Map<string, string>()
for (const group of sidebarGroups) {
  for (const item of group.items) {
    pageToSection.set(item.link.replace(/^\//, ''), group.text)
  }
}

export default withMermaid({
  title: 'Levelrail',
  description,

  // TODO: revisit once glinr.com/levelrail (a path, not this subdomain)
  // becomes possible, per the root CLAUDE.md's stated long-term target.
  sitemap: {
    hostname: siteUrl,
  },

  head: [
    [
      'meta',
      {
        property: 'og:image',
        content: `${siteUrl}/assets/screenshots/app-overview.png`,
      },
    ],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    [
      'meta',
      {
        name: 'twitter:image',
        content: `${siteUrl}/assets/screenshots/app-overview.png`,
      },
    ],
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
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
      }),
    ],
  ],

  cleanUrls: true,
  appearance: 'dark',

  // A handful of docs link up to files outside docs/ (root README.md,
  // CHANGELOG.md, /adr) that exist in the repo but sit outside this
  // site's srcDir, by design: docs/README.md documents that these pages
  // are meant to render correctly on GitHub too, not just in this site.
  ignoreDeadLinks: [/\.\.\//],

  themeConfig: {
    logo: undefined,

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
  },

  // Per-page canonical link and BreadcrumbList: VitePress doesn't add
  // either by default. Skips index.md (the home layout has no
  // meaningful breadcrumb) and any page pageToSection doesn't
  // recognize (docs/README.md, ADRs reached via ../adr, etc.).
  transformPageData(pageData) {
    return changelogPageData(pageData)
  },

  buildEnd: (config) => buildEnd(config, siteUrl),

  transformHead({ pageData }) {
    const path = pageData.relativePath.replace(/\.md$/, '').replace(/(^|\/)index$/, '$1')
    const canonicalUrl = `${siteUrl}/${path}`
    const title = pageData.frontmatter.title || pageData.title || 'Levelrail'
    const pageDescription = pageData.frontmatter.description || pageData.description || description
    const head: [string, Record<string, string>, string?][] = [
      ['link', { rel: 'canonical', href: canonicalUrl }],
      ['meta', { property: 'og:type', content: pageData.params?.tag ? 'article' : 'website' }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: pageDescription }],
      ['meta', { property: 'og:url', content: canonicalUrl }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: pageDescription }],
      ...changelogHead(pageData, siteUrl),
    ]

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
      primaryBorderColor: '#f59e0b',
      lineColor: '#a1a1aa',
      secondaryColor: '#10141c',
      tertiaryColor: '#0b0e14',
    },
  },
})
