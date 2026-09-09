import { defineConfig } from 'vitepress'

const description =
  'A self-hosted deployment platform whose agent talks to Docker’s own Engine API directly, ' +
  'no SSH or CLI shelling, with metrics and log storage built into the core.'

export default defineConfig({
  title: 'Levelrail',
  description,

  // TODO: replace with the real subdomain once chosen, then again with
  // glinr.com/levelrail once that move happens.
  sitemap: {
    hostname: 'https://levelrail.example.com',
  },

  head: [
    ['meta', { name: 'description', content: description }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'Levelrail' }],
    ['meta', { property: 'og:description', content: description }],
    [
      'meta',
      {
        property: 'og:image',
        content: '/assets/screenshots/app-overview.png',
      },
    ],
  ],

  cleanUrls: true,

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
            text: 'Migrating from Coolify and Dokploy',
            link: '/migrating-from-coolify-and-dokploy',
          },
          { text: 'Screenshots', link: '/screenshots' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'App spec reference', link: '/app-spec-reference' },
          { text: 'Feature catalog', link: '/feature-catalog' },
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
