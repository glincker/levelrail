import { mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const repo = 'glincker/levelrail'
export const product = 'Levelrail'

export interface ReleaseEntry {
  tag: string
  slug: string
  date: string
  published: string
  url: string
  prerelease: boolean
  body: string
  highlights: string[]
  title: string
  description: string
}

const here = dirname(fileURLToPath(import.meta.url))
const cacheFile = resolve(here, 'cache/releases.json')
const changelogFile = resolve(here, '../../CHANGELOG.md')
const cacheTtlMs = 10 * 60 * 1000

export function slugFor(tag: string): string {
  return tag.toLowerCase().replace(/[.+]/g, '-')
}

function plain(line: string): string {
  return line
    .replace(/^[-*]\s+/, '')
    .replace(/\s*\(\[[^\]]*\]\([^)]*\)\)/g, '')
    .replace(/\s+by @\S+$/, '')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\*\*([^*]*)\*\*/g, '$1')
    .replace(/`/g, '')
    .trim()
    .replace(/^\p{Ll}/u, (c) => c.toUpperCase())
}

function bulletsUnder(body: string, heading: RegExp): string[] {
  const lines = body.split('\n')
  const start = lines.findIndex((l) => heading.test(l))
  if (start < 0) return []
  const out: string[] = []
  for (const line of lines.slice(start + 1)) {
    if (/^#{1,6}\s/.test(line) || line.startsWith('<details')) break
    if (/^[-*]\s/.test(line)) out.push(plain(line))
  }
  return out
}

export function highlightsOf(body: string): string[] {
  for (const h of [/^##\s+Highlights/i, /^###\s+Features/i, /^###\s+(Bug fixes|Security)/i]) {
    const found = bulletsUnder(body, h)
    if (found.length) return found
  }
  return []
}

function clip(text: string, max: number): string {
  if (text.length <= max) return text
  const cut = text.slice(0, max - 3)
  return cut.slice(0, cut.lastIndexOf(' ')).replace(/[,;:]$/, '') + '...'
}

function entry(tag: string, published: string, url: string, prerelease: boolean, body: string): ReleaseEntry {
  const date = published.slice(0, 10)
  const highlights = highlightsOf(body)
  const summary = highlights.length ? highlights.slice(0, 2).join(', ') : 'release notes'
  return {
    tag,
    slug: slugFor(tag),
    date,
    published,
    url,
    prerelease,
    body,
    highlights,
    title: clip(`${product} ${tag}: ${summary}`, 70),
    description: clip(
      `${product} ${tag} release notes (${date}). ` +
        (highlights.length ? highlights.join('; ') + '.' : 'Changes, install and upgrade steps, verification.'),
      160,
    ),
  }
}

async function fromGitHub(): Promise<ReleaseEntry[]> {
  const headers: Record<string, string> = { Accept: 'application/vnd.github+json' }
  const token = process.env.GITHUB_TOKEN || process.env.GH_TOKEN
  if (token) headers.Authorization = `Bearer ${token}`
  const all: ReleaseEntry[] = []
  for (let page = 1; page < 10; page++) {
    const res = await fetch(`https://api.github.com/repos/${repo}/releases?per_page=100&page=${page}`, { headers })
    if (!res.ok) throw new Error(`GitHub releases API: ${res.status}`)
    const batch = (await res.json()) as {
      tag_name: string
      published_at: string
      html_url: string
      prerelease: boolean
      draft: boolean
      body: string | null
    }[]
    for (const r of batch) {
      if (r.draft) continue
      all.push(entry(r.tag_name, r.published_at, r.html_url, r.prerelease, r.body ?? ''))
    }
    if (batch.length < 100) break
  }
  return all
}

function fromChangelog(): ReleaseEntry[] {
  const text = readFileSync(changelogFile, 'utf8')
  const parts = text.split(/^## \[?(\d[^\]\s]*)\]?(?:\([^)]*\))?\s*\((\d{4}-\d{2}-\d{2})\)\s*$/m)
  const out: ReleaseEntry[] = []
  for (let i = 1; i < parts.length; i += 3) {
    const tag = `v${parts[i]}`
    out.push(
      entry(tag, parts[i + 1], `https://github.com/${repo}/releases/tag/${tag}`, tag.includes('-'), (parts[i + 2] ?? '').trim()),
    )
  }
  return out
}

let pending: Promise<ReleaseEntry[]> | undefined

/** Releases newest first, from the GitHub API, falling back to CHANGELOG.md. */
export function loadReleases(): Promise<ReleaseEntry[]> {
  pending ??= (async () => {
    try {
      if (Date.now() - statSync(cacheFile).mtimeMs < cacheTtlMs) {
        return JSON.parse(readFileSync(cacheFile, 'utf8')) as ReleaseEntry[]
      }
    } catch {
      // No fresh cache: fetch below.
    }
    let releases: ReleaseEntry[]
    try {
      releases = await fromGitHub()
    } catch (error) {
      console.warn(`[changelog] GitHub API unavailable, using CHANGELOG.md:`, error)
      releases = fromChangelog()
    }
    releases.sort((a, b) => b.published.localeCompare(a.published))
    try {
      mkdirSync(dirname(cacheFile), { recursive: true })
      writeFileSync(cacheFile, JSON.stringify(releases))
    } catch {
      // Cache is an optimization only.
    }
    return releases
  })()
  return pending
}

/** Release body adjusted for the docs site: linked mentions, no Vue interpolation. */
export function docsBody(body: string): string {
  const linked = body.replace(/(^|[\s(])@([A-Za-z0-9-]+(?:\[bot\])?)/g, (_m, pre: string, login: string) =>
    login.endsWith('[bot]') ? `${pre}${login}` : `${pre}[@${login}](https://github.com/${login})`,
  )
  return `::: v-pre\n${linked}\n:::\n`
}
