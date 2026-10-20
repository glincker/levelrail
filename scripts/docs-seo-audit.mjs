#!/usr/bin/env node
// Docs SEO audit: frontmatter, nav orphans, links, headings, alt text. No deps.
import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs'
import { join, relative, dirname, resolve } from 'node:path'

const strict = process.argv.includes('--strict')
const root = resolve(new URL('../docs', import.meta.url).pathname)
const TITLE = [50, 60]
const DESC = [140, 160]

const files = []
const walk = (d) => {
  for (const n of readdirSync(d)) {
    if (n === 'node_modules' || n.startsWith('.') || n === 'public') continue
    const p = join(d, n)
    const st = statSync(p)
    if (st.isDirectory()) { if (n !== 'assets') walk(p) } else if (/\.mdx?$/.test(n)) files.push(p)
  }
}
walk(root)

const ignored = new Set(
  existsSync(join(root, '.mintignore'))
    ? readFileSync(join(root, '.mintignore'), 'utf8').split('\n').map((l) => l.trim()).filter(Boolean)
    : [],
)
const slugOf = (f) => relative(root, f).replace(/\.mdx?$/, '')
const pages = files.filter((f) => !ignored.has(relative(root, f)))
const slugs = new Set(pages.map(slugOf))

const nav = new Set()
const collect = (n) => {
  if (typeof n === 'string') return nav.add(n.replace(/^\//, ''))
  if (Array.isArray(n)) return n.forEach(collect)
  if (n && typeof n === 'object') for (const k of ['pages', 'groups', 'tabs', 'anchors', 'dropdowns', 'versions', 'languages', 'menu']) if (n[k]) collect(n[k])
}
const cfg = JSON.parse(readFileSync(join(root, 'docs.json'), 'utf8'))
collect(cfg.navigation)
const redirects = new Set((cfg.redirects || []).map((r) => r.source.replace(/^\//, '')))

const fm = (src) => {
  const m = src.match(/^---\n([\s\S]*?)\n---\n?/)
  const out = {}
  if (!m) return { out, body: src }
  for (const line of m[1].split('\n')) {
    const k = line.match(/^([A-Za-z:_-]+):\s*(.*)$/)
    if (k) out[k[1]] = k[2].trim().replace(/^(["'])([\s\S]*)\1$/, '$2')
  }
  return { out, body: src.slice(m[0].length) }
}

const issues = []
const add = (page, level, msg) => issues.push({ page, level, msg })
const titles = new Map()
const counts = { pages: pages.length, noTitle: 0, titleLen: 0, noDesc: 0, descLen: 0, dupTitle: 0, orphan: 0, broken: 0, h1: 0, alt: 0 }

for (const f of pages) {
  const slug = slugOf(f)
  const src = readFileSync(f, 'utf8')
  const { out, body } = fm(src)
  if (!out.title) { counts.noTitle++; add(slug, 'error', 'missing title') }
  else {
    const l = out.title.length
    if (l < TITLE[0] || l > TITLE[1]) { counts.titleLen++; add(slug, 'warn', `title length ${l}`) }
    const key = out.title.toLowerCase()
    titles.set(key, [...(titles.get(key) || []), slug])
  }
  if (!out.description) { counts.noDesc++; add(slug, 'error', 'missing description') }
  else {
    const l = out.description.length
    if (l < DESC[0] || l > DESC[1]) { counts.descLen++; add(slug, l > DESC[1] ? 'error' : 'warn', `description length ${l}`) }
  }
  if (!nav.has(slug) && !redirects.has(slug)) { counts.orphan++; add(slug, 'warn', 'not in docs.json navigation') }

  const noCode = body.replace(/```[\s\S]*?```/g, '').replace(/`[^`\n]*`/g, '')
  const h1s = (noCode.match(/^# .+$/gm) || []).length + (out.title ? 1 : 0)
  if (h1s > 1) { counts.h1++; add(slug, 'warn', `${h1s} H1s (frontmatter title counts as one)`) }

  for (const m of noCode.matchAll(/!\[([^\]]*)\]\(([^)]+)\)/g)) if (!m[1].trim()) { counts.alt++; add(slug, 'error', `image without alt: ${m[2]}`) }
  for (const m of noCode.matchAll(/<img\b[^>]*>/g)) if (!/\balt=("[^"]+"|'[^']+')/.test(m[0])) { counts.alt++; add(slug, 'error', 'img tag without alt') }

  const links = [...noCode.matchAll(/(?<!!)\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g)].map((m) => m[1]).concat([...noCode.matchAll(/href="([^"]+)"/g)].map((m) => m[1]))
  for (const raw of links) {
    if (/^(https?:|mailto:|#|tel:)/.test(raw)) continue
    const path = raw.split('#')[0].split('?')[0]
    if (!path) continue
    let target = path.startsWith('/') ? path.slice(1) : join(dirname(slug), path)
    target = target.replace(/\.mdx?$/, '').replace(/\/$/, '')
    if (slugs.has(target) || slugs.has(target + '/index') || redirects.has(target)) continue
    if (/^(assets|public)\//.test(target) || existsSync(join(root, target)) || existsSync(join(root, 'public', target))) continue
    counts.broken++; add(slug, 'error', `broken link ${raw}`)
  }
}
for (const [t, s] of titles) if (s.length > 1) { counts.dupTitle += s.length; add(s.join(', '), 'error', `duplicate title "${t}"`) }

const rows = [
  ['pages', counts.pages], ['missing title', counts.noTitle], ['title outside 50-60', counts.titleLen],
  ['missing description', counts.noDesc], ['description outside 140-160', counts.descLen], ['duplicate titles', counts.dupTitle],
  ['orphan pages', counts.orphan], ['broken internal links', counts.broken], ['multiple H1', counts.h1], ['images without alt', counts.alt],
]
const w = Math.max(...rows.map((r) => r[0].length))
for (const [k, v] of rows) console.log(`${k.padEnd(w)}  ${v}`)
if (process.argv.includes('--verbose')) for (const i of issues) console.log(`${i.level.padEnd(5)} ${i.page}: ${i.msg}`)
const errors = issues.filter((i) => i.level === 'error').length
console.log(`errors ${errors}, warnings ${issues.length - errors}`)
if (strict && errors > 0) process.exit(1)
