import { defineConfig } from 'vitepress'

const description =
  'A self-hosted deployment platform whose agent talks to Docker’s own Engine API directly, ' +
  'no SSH or CLI shelling, with metrics and log storage built into the core.'

export default defineConfig({
  title: 'Levelrail',
  description,

  // TODO: revisit once glinr.com/levelrail (a path, not this subdomain)
  // becomes possible, per the root CLAUDE.md's stated long-term target.
  sitemap: {
    hostname: 'https://levelrail.glinr.com',
  },

  head: [
    ['meta', { name: 'description', content: description }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'Levelrail' }],
    ['meta', { property: 'og:description', content: description }],
    ['meta', { property: 'og:url', content: 'https://levelrail.glinr.com/' }],
    [
      'meta',
      {
        property: 'og:image',
        content: 'https://levelrail.glinr.com/assets/screenshots/app-overview.png',
      },
    ],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'twitter:title', content: 'Levelrail' }],
    ['meta', { name: 'twitter:description', content: description }],
    [
      'meta',
      {
        name: 'twitter:image',
        content: 'https://levelrail.glinr.com/assets/screenshots/app-overview.png',
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
        url: 'https://levelrail.glinr.com/',
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
      { text: 'Roadmap', link: '/roadmap' },
    ],

    sidebar: [
      {
        text: 'Tutorials',
        items: [{ text: 'Getting started', link: '/getting-started' }],
      },
      {
        text: 'How-to guides',
        items: [
          { text: 'Installing', link: '/installing' },
          { text: 'Docker', link: '/docker' },
          { text: 'Domains and ingress', link: '/domains-and-ingress' },
          { text: 'ACME verification runbook', link: '/acme-verification-runbook' },
          { text: 'Feature flags', link: '/feature-flags' },
          { text: 'Master key rotation', link: '/master-key-rotation' },
          {
            text: 'Migrating from Coolify, Dokploy, or CapRover',
            link: '/migrating-from-coolify-dokploy-and-caprover',
          },
          { text: 'Deploying from GitHub Actions', link: '/github-actions' },
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
          { text: 'Templates and registry', link: '/templates-and-registry' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'App spec reference', link: '/app-spec-reference' },
          { text: 'Feature catalog', link: '/feature-catalog' },
          { text: 'CLI reference', link: '/cli-reference' },
          { text: 'API reference', link: '/api-reference' },
        ],
      },
      {
        text: 'Explanation',
        items: [
          { text: 'Architecture', link: '/architecture' },
          { text: 'Comparison', link: '/comparison' },
        ],
      },
      {
        text: 'Design proposals',
        items: [
          {
            text: 'Git provider integrations',
            link: '/design/git-provider-integrations',
          },
        ],
      },
      {
        text: 'Status',
        items: [{ text: 'Roadmap', link: '/roadmap' }],
      },
      {
        text: 'Docs index',
        items: [{ text: 'Overview', link: '/README' }],
      },
    ],

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
})
