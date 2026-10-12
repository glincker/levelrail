import { defineConfig } from 'vitepress'
import { faqItems } from './theme/faqData'

const description =
  'A self-hosted deployment platform whose agent talks to Docker’s own Engine API directly, ' +
  'no SSH or CLI shelling, with metrics and log storage built into the core.'

const siteUrl = 'https://levelrail.com'
// Fixed-path assets sit behind a CDN, so bump this when favicon content changes.
const faviconVersion = 4

export default defineConfig({
  title: 'Levelrail',
  description,
  cleanUrls: true,
  appearance: 'dark',
  lastUpdated: false,

  sitemap: { hostname: siteUrl },

  head: [
    ['meta', { property: 'og:image', content: `${siteUrl}/assets/og-home.png` }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'theme-color', content: '#0b0e14' }],
    ['link', { rel: 'icon', href: `/favicon.svg?v=${faviconVersion}`, type: 'image/svg+xml' }],
    ['link', { rel: 'icon', href: `/favicon-96x96.png?v=${faviconVersion}`, type: 'image/png', sizes: '96x96' }],
    ['link', { rel: 'icon', href: `/favicon.ico?v=${faviconVersion}`, sizes: '48x48' }],
    ['link', { rel: 'apple-touch-icon', href: `/apple-touch-icon.png?v=${faviconVersion}`, sizes: '180x180' }],
    ['meta', { name: 'apple-mobile-web-app-title', content: 'Levelrail' }],
    ['link', { rel: 'manifest', href: `/site.webmanifest?v=${faviconVersion}` }],
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
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
        license: 'https://www.apache.org/licenses/LICENSE-2.0',
        url: `${siteUrl}/`,
        codeRepository: 'https://github.com/glincker/levelrail',
        publisher: { '@type': 'Organization', name: 'GLINCKER', url: 'https://glincker.com' },
      }),
    ],
    [
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'FAQPage',
        mainEntity: faqItems.map((q) => ({
          '@type': 'Question',
          name: q.question,
          acceptedAnswer: { '@type': 'Answer', text: q.answer },
        })),
      }),
    ],
  ],

  themeConfig: {
    socialLinks: [{ icon: 'github', link: 'https://github.com/glincker/levelrail' }],
  },
})
