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
      { text: 'Getting started', link: '/getting-started' },
      { text: 'Demo', link: '/demo' },
      { text: 'Pricing', link: '/pricing' },
      { text: 'Case studies', link: '/case-studies' },
      { text: 'Roadmap', link: '/roadmap' },
      { text: 'Changelog', link: '/changelog/' },
    ],
  },
  {
    heading: 'Compare',
    links: [
      { text: 'Full comparison', link: '/comparison' },
      { text: 'Coolify alternative', link: '/coolify-alternative' },
      { text: 'Dokploy alternative', link: '/dokploy-alternative' },
      { text: 'CapRover alternative', link: '/caprover-alternative' },
      { text: 'Dokku alternative', link: '/dokku-alternative' },
      { text: 'Kamal alternative', link: '/kamal-alternative' },
      { text: 'Vercel alternative', link: '/vercel-alternative' },
      { text: 'Heroku alternative', link: '/heroku-alternative' },
      { text: 'Railway alternative', link: '/railway-alternative' },
    ],
  },
  {
    heading: 'Learn',
    links: [
      { text: 'Installing', link: '/installing' },
      { text: 'CLI reference', link: '/cli-reference' },
      { text: 'API reference', link: '/api-reference' },
      { text: 'App spec reference', link: '/app-spec-reference' },
      { text: 'Developers', link: '/developers' },
      { text: 'Security overview', link: '/security' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
    ],
  },
  {
    heading: 'Company',
    links: [
      { text: 'About', link: '/about' },
      { text: 'Contact', link: '/contact' },
      { text: 'Contribute', link: '/contribute' },
      { text: 'Privacy and data', link: '/privacy' },
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
  { text: 'Terms', link: '/terms' },
  { text: 'Privacy policy', link: '/privacy-policy' },
  { text: 'Cookies', link: '/cookies' },
  { text: 'License', link: '/license' },
]
