export interface FooterLink {
  text: string
  link: string
  external?: boolean
}

export interface FooterColumn {
  heading: string
  links: FooterLink[]
}

const REPO = 'https://github.com/glincker/levelrail'

// One list drives the whole footer: add a link here and it appears in the right column.
export const footerColumns: FooterColumn[] = [
  {
    heading: 'Product',
    links: [
      { text: 'Getting started', link: 'https://docs.levelrail.com/getting-started' },
      { text: 'Demo', link: 'https://docs.levelrail.com/demo' },
      { text: 'Pricing', link: 'https://docs.levelrail.com/pricing' },
      { text: 'Case studies', link: 'https://docs.levelrail.com/case-studies' },
      { text: 'Roadmap', link: 'https://docs.levelrail.com/roadmap' },
      { text: 'Changelog', link: 'https://docs.levelrail.com/changelog/' },
    ],
  },
  {
    heading: 'Compare',
    links: [
      { text: 'Full comparison', link: 'https://docs.levelrail.com/comparison' },
      { text: 'Coolify alternative', link: 'https://docs.levelrail.com/coolify-alternative' },
      { text: 'Dokploy alternative', link: 'https://docs.levelrail.com/dokploy-alternative' },
      { text: 'CapRover alternative', link: 'https://docs.levelrail.com/caprover-alternative' },
      { text: 'Dokku alternative', link: 'https://docs.levelrail.com/dokku-alternative' },
      { text: 'Kamal alternative', link: 'https://docs.levelrail.com/kamal-alternative' },
      { text: 'Vercel alternative', link: 'https://docs.levelrail.com/vercel-alternative' },
      { text: 'Heroku alternative', link: 'https://docs.levelrail.com/heroku-alternative' },
      { text: 'Railway alternative', link: 'https://docs.levelrail.com/railway-alternative' },
    ],
  },
  {
    heading: 'Learn',
    links: [
      { text: 'Installing', link: 'https://docs.levelrail.com/installing' },
      { text: 'CLI reference', link: 'https://docs.levelrail.com/cli-reference' },
      { text: 'API reference', link: 'https://docs.levelrail.com/api-reference' },
      { text: 'App spec reference', link: 'https://docs.levelrail.com/app-spec-reference' },
      { text: 'Developers', link: 'https://docs.levelrail.com/developers' },
      { text: 'Security overview', link: 'https://docs.levelrail.com/security' },
      { text: 'Troubleshooting', link: 'https://docs.levelrail.com/troubleshooting' },
    ],
  },
  {
    heading: 'Company',
    links: [
      { text: 'About', link: 'https://docs.levelrail.com/about' },
      { text: 'Contact', link: 'https://docs.levelrail.com/contact' },
      { text: 'Contribute', link: 'https://github.com/glincker/levelrail/blob/main/CONTRIBUTING.md' },
      { text: 'Privacy and data', link: 'https://docs.levelrail.com/privacy' },
    ],
  },
  {
    heading: 'Community',
    links: [
      { text: 'GitHub', link: REPO, external: true },
      { text: 'Issues', link: `${REPO}/issues`, external: true },
      { text: 'Discussions', link: `${REPO}/discussions`, external: true },
      { text: 'Discord', link: 'https://discord.gg/Ar5pcaZB99', external: true },
      { text: 'support@levelrail.com', link: 'mailto:support@levelrail.com', external: true },
    ],
  },
]

export const footerLegalLinks: FooterLink[] = [
  { text: 'Terms', link: 'https://docs.levelrail.com/terms' },
  { text: 'Privacy policy', link: 'https://docs.levelrail.com/privacy-policy' },
  { text: 'Cookies', link: 'https://docs.levelrail.com/cookies' },
  { text: 'License', link: 'https://github.com/glincker/levelrail/blob/main/LICENSE' },
]
