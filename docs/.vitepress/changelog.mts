import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { createMarkdownRenderer, type HeadConfig, type PageData, type SiteConfig } from 'vitepress'
import { docsBody, loadReleases, product, repo, type ReleaseEntry } from './releases.mts'

let byTag = new Map<string, { entry: ReleaseEntry; prev?: ReleaseEntry; next?: ReleaseEntry }>()

async function index() {
  if (byTag.size) return byTag
  const releases = await loadReleases()
  byTag = new Map(
    releases.map((entry, i) => [entry.tag, { entry, next: releases[i - 1], prev: releases[i + 1] }]),
  )
  return byTag
}

// Dynamic route pages share one markdown file, so their title, description,
// and prev/next links are filled in per release here.
export async function changelogPageData(pageData: PageData) {
  const tag = pageData.params?.tag as string | undefined
  if (!tag) return
  const found = (await index()).get(tag)
  if (!found) return
  const { entry, prev, next } = found
  pageData.title = entry.title
  pageData.titleTemplate = ':title'
  pageData.description = entry.description
  pageData.frontmatter.title = entry.title
  pageData.frontmatter.description = entry.description
  pageData.frontmatter.prev = prev ? { text: prev.tag, link: `/changelog/${prev.slug}` } : false
  pageData.frontmatter.next = next ? { text: next.tag, link: `/changelog/${next.slug}` } : false
}

export function changelogHead(pageData: PageData, siteUrl: string): HeadConfig[] {
  const tag = pageData.params?.tag as string | undefined
  if (!tag) {
    return pageData.relativePath === 'changelog/index.md' ? [feedLink(siteUrl)] : []
  }
  const found = byTag.get(tag)
  if (!found) return []
  const { entry, prev, next } = found
  const url = `${siteUrl}/changelog/${entry.slug}`
  const head: HeadConfig[] = [
    feedLink(siteUrl),
    ['meta', { property: 'article:published_time', content: entry.published }],
  ]
  if (prev) head.push(['link', { rel: 'prev', href: `${siteUrl}/changelog/${prev.slug}` }])
  if (next) head.push(['link', { rel: 'next', href: `${siteUrl}/changelog/${next.slug}` }])
  head.push([
    'script',
    { type: 'application/ld+json' },
    JSON.stringify({
      '@context': 'https://schema.org',
      '@graph': [
        {
          '@type': 'TechArticle',
          headline: entry.title,
          description: entry.description,
          datePublished: entry.published,
          dateModified: entry.published,
          url,
          mainEntityOfPage: url,
          author: { '@type': 'Organization', name: 'GLINCKER', url: 'https://github.com/glincker' },
          about: {
            '@type': 'SoftwareApplication',
            name: product,
            softwareVersion: entry.tag.replace(/^v/, ''),
            applicationCategory: 'DeveloperApplication',
            operatingSystem: 'Linux',
            releaseNotes: url,
            downloadUrl: entry.url,
            codeRepository: `https://github.com/${repo}`,
          },
        },
        {
          '@type': 'BreadcrumbList',
          itemListElement: [
            { '@type': 'ListItem', position: 1, name: product, item: `${siteUrl}/` },
            { '@type': 'ListItem', position: 2, name: 'Changelog', item: `${siteUrl}/changelog/` },
            { '@type': 'ListItem', position: 3, name: entry.tag, item: url },
          ],
        },
      ],
    }),
  ])
  return head
}

function feedLink(siteUrl: string): HeadConfig {
  return [
    'link',
    { rel: 'alternate', type: 'application/atom+xml', title: `${product} releases`, href: `${siteUrl}/changelog/feed.xml` },
  ]
}

function xml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

/** Writes the Atom feed for the changelog into the build output. */
export async function buildEnd(config: SiteConfig, siteUrl: string) {
  const releases = (await loadReleases()).slice(0, 50)
  const md = await createMarkdownRenderer(config.srcDir, config.markdown, config.site.base, config.logger)
  const feedUrl = `${siteUrl}/changelog/feed.xml`
  const entries = releases.map((r) => {
    const url = `${siteUrl}/changelog/${r.slug}`
    return [
      '  <entry>',
      `    <id>${url}</id>`,
      `    <title>${xml(r.title)}</title>`,
      `    <link rel="alternate" type="text/html" href="${url}"/>`,
      `    <published>${r.published}</published>`,
      `    <updated>${r.published}</updated>`,
      `    <summary>${xml(r.description)}</summary>`,
      `    <content type="html">${xml(md.render(docsBody(r.body)))}</content>`,
      '  </entry>',
    ].join('\n')
  })
  const feed = [
    '<?xml version="1.0" encoding="utf-8"?>',
    '<feed xmlns="http://www.w3.org/2005/Atom">',
    `  <id>${siteUrl}/changelog/</id>`,
    `  <title>${product} releases</title>`,
    `  <subtitle>Release notes for ${product}, the self-hosted deployment platform.</subtitle>`,
    `  <link rel="self" type="application/atom+xml" href="${feedUrl}"/>`,
    `  <link rel="alternate" type="text/html" href="${siteUrl}/changelog/"/>`,
    `  <updated>${releases[0]?.published ?? new Date().toISOString()}</updated>`,
    `  <author><name>GLINCKER</name><uri>https://github.com/glincker</uri></author>`,
    ...entries,
    '</feed>',
    '',
  ].join('\n')
  writeFileSync(join(config.outDir, 'changelog', 'feed.xml'), feed)
}
