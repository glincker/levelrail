// Rewrites the version tracker table in VERSIONING.md from CHANGELOG.md, which
// release-please maintains, so the table follows every release with no hand edit.
// Env: VERSIONS_CHANGELOG, VERSIONS_TARGET (paths), VERSIONS_REPO (owner/name).
import fs from 'node:fs'
import { pathToFileURL } from 'node:url'

export const BEGIN = '<!-- versions:begin -->'
export const END = '<!-- versions:end -->'

const HEADING = /^## \[([^\]]+)\]\([^)]*\) \((\d{4}-\d{2}-\d{2})\)/

export function parseReleases(changelog) {
  const out = []
  for (const line of changelog.split('\n')) {
    const m = HEADING.exec(line)
    if (m) out.push({ version: m[1], date: m[2] })
  }
  return out
}

// Status stays blank: the support policy is a draft and states none per version.
export function renderTable(releases, repo) {
  const rows = releases.map((r, i) => {
    const kind = /-/.test(r.version) ? 'Pre-release' : i === 0 ? 'Latest' : ''
    return `| ${r.version} | ${r.date} | ${kind} | [Release notes](https://github.com/${repo}/releases/tag/v${r.version}) |`
  })
  return ['| Version | Date | Status | Notes |', '| --- | --- | --- | --- |', ...rows].join('\n')
}

export function splice(doc, table) {
  const a = doc.indexOf(BEGIN)
  const b = doc.indexOf(END)
  if (a < 0 || b < a) throw new Error('versions markers missing')
  return `${doc.slice(0, a + BEGIN.length)}\n${table}\n${doc.slice(b)}`
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) {
  const cl = process.env.VERSIONS_CHANGELOG ?? 'CHANGELOG.md'
  const target = process.env.VERSIONS_TARGET ?? 'VERSIONING.md'
  const repo = process.env.VERSIONS_REPO ?? 'glincker/levelrail'
  const table = renderTable(parseReleases(fs.readFileSync(cl, 'utf8')), repo)
  fs.writeFileSync(target, splice(fs.readFileSync(target, 'utf8'), table))
}
