import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { createContentLoader, type HeadConfig, type SiteConfig } from 'vitepress'
import { loadReleases, product } from './releases.mts'

interface FeedItem {
  title: string
  summary: string
  url: string
  date: string
}

const author = { name: 'GLINCKER', url: 'https://github.com/glincker' }
const tagline = `News and releases for ${product}, the self-hosted deployment platform.`

function xml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

/** Autodiscovery links so feed readers and crawlers find the site feeds. */
export function feedHead(siteUrl: string): HeadConfig[] {
  return [
    ['link', { rel: 'alternate', type: 'application/rss+xml', title: `${product} announcements`, href: `${siteUrl}/feed.xml` }],
    ['link', { rel: 'alternate', type: 'application/atom+xml', title: `${product} announcements (Atom)`, href: `${siteUrl}/atom.xml` }],
    ['link', { rel: 'alternate', type: 'application/feed+json', title: `${product} announcements (JSON)`, href: `${siteUrl}/feed.json` }],
  ]
}

async function collect(siteUrl: string): Promise<FeedItem[]> {
  const posts = await createContentLoader('announcements/*.md').load()
  const items: FeedItem[] = posts
    .filter((p) => !p.url.endsWith('/announcements/') && p.frontmatter.date)
    .map((p) => ({
      title: String(p.frontmatter.title),
      summary: String(p.frontmatter.description ?? ''),
      url: `${siteUrl}${p.url}`,
      date: new Date(p.frontmatter.date).toISOString(),
    }))
  const releases = (await loadReleases()).slice(0, 20)
  for (const r of releases) {
    items.push({ title: r.title, summary: r.description, url: `${siteUrl}/changelog/${r.slug}`, date: new Date(r.published).toISOString() })
  }
  return items.sort((a, b) => b.date.localeCompare(a.date))
}

/** Writes /feed.xml (RSS 2.0), /atom.xml and /feed.json covering announcements and releases. */
export async function writeFeeds(config: SiteConfig, siteUrl: string) {
  const items = await collect(siteUrl)
  const updated = items[0]?.date ?? new Date().toISOString()

  const rss = [
    '<?xml version="1.0" encoding="utf-8"?>',
    '<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">',
    '<channel>',
    `<title>${xml(product)} announcements</title>`,
    `<link>${siteUrl}/announcements/</link>`,
    `<description>${xml(tagline)}</description>`,
    '<language>en</language>',
    `<lastBuildDate>${new Date(updated).toUTCString()}</lastBuildDate>`,
    `<atom:link href="${siteUrl}/feed.xml" rel="self" type="application/rss+xml"/>`,
    ...items.map((i) =>
      `<item><title>${xml(i.title)}</title><link>${i.url}</link><guid isPermaLink="true">${i.url}</guid><pubDate>${new Date(i.date).toUTCString()}</pubDate><description>${xml(i.summary)}</description></item>`),
    '</channel>',
    '</rss>',
    '',
  ].join('\n')

  const atom = [
    '<?xml version="1.0" encoding="utf-8"?>',
    '<feed xmlns="http://www.w3.org/2005/Atom">',
    `<id>${siteUrl}/announcements/</id>`,
    `<title>${xml(product)} announcements</title>`,
    `<subtitle>${xml(tagline)}</subtitle>`,
    `<link rel="self" type="application/atom+xml" href="${siteUrl}/atom.xml"/>`,
    `<link rel="alternate" type="text/html" href="${siteUrl}/announcements/"/>`,
    `<updated>${updated}</updated>`,
    `<author><name>${author.name}</name><uri>${author.url}</uri></author>`,
    ...items.map((i) =>
      `<entry><id>${i.url}</id><title>${xml(i.title)}</title><link rel="alternate" type="text/html" href="${i.url}"/><published>${i.date}</published><updated>${i.date}</updated><summary>${xml(i.summary)}</summary></entry>`),
    '</feed>',
    '',
  ].join('\n')

  const json = JSON.stringify(
    {
      version: 'https://jsonfeed.org/version/1.1',
      title: `${product} announcements`,
      home_page_url: `${siteUrl}/announcements/`,
      feed_url: `${siteUrl}/feed.json`,
      description: tagline,
      authors: [{ name: author.name, url: author.url }],
      items: items.map((i) => ({ id: i.url, url: i.url, title: i.title, summary: i.summary, date_published: i.date })),
    },
    null,
    2,
  )

  writeFileSync(join(config.outDir, 'feed.xml'), rss)
  writeFileSync(join(config.outDir, 'atom.xml'), atom)
  writeFileSync(join(config.outDir, 'feed.json'), json + '\n')
}
