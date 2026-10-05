export interface RelatedLink {
  text: string
  link: string
}

// Add a landing page here once and every other landing page links to it.
export const landingRelatedLinks: RelatedLink[] = [
  { text: 'Coolify alternative', link: '/coolify-alternative' },
  { text: 'Dokploy alternative', link: '/dokploy-alternative' },
  { text: 'CapRover alternative', link: '/caprover-alternative' },
  { text: 'Dokku alternative', link: '/dokku-alternative' },
  { text: 'Kamal alternative', link: '/kamal-alternative' },
  { text: 'Vercel alternative', link: '/vercel-alternative' },
  { text: 'Heroku alternative', link: '/heroku-alternative' },
  { text: 'Railway alternative', link: '/railway-alternative' },
  { text: 'Self-hosted PaaS guide', link: '/self-hosted-paas' },
  { text: 'Self-host Next.js', link: '/self-host-nextjs' },
  { text: 'Zero-downtime deploys', link: '/zero-downtime-deploys' },
  { text: 'Preview environments', link: '/preview-environments' },
  { text: 'Pricing', link: '/pricing' },
  { text: 'Privacy', link: '/privacy' },
  { text: 'Demo', link: '/demo' },
  { text: 'Case studies', link: '/case-studies' },
  { text: 'Contribute', link: '/contribute' },
  { text: 'Full comparison', link: '/comparison' },
]
