#!/usr/bin/env node
// Diff-aware check: does a commit type match what its diff touches.
// Report-only by default; set SEMVER_ENFORCE=1 to exit non-zero on findings.
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const csv = (name, fallback) => (process.env[name] ?? fallback).split(',').map((s) => s.trim()).filter(Boolean)

// Path prefixes are env-tunable so the rules follow repo layout changes.
export const rules = () => ({
  userVisible: csv('SEMVER_USER_VISIBLE', 'internal/api/,cmd/,web/src/,templates/,internal/iac/schema/'),
  surface: csv('SEMVER_STABLE_SURFACE', 'internal/iac/schema/,docs/schemas/,docs/api-reference/,docs/openapi'),
  newSurface: csv('SEMVER_NEW_SURFACE', 'internal/api/,cmd/,internal/iac/schema/'),
})

const headerRe = /^(\w+)(\([^)]*\))?(!)?: (.+)$/

export function parseCommit(message) {
  const [subject, ...rest] = message.split('\n')
  const m = headerRe.exec(subject)
  if (!m) return { subject, type: null, breaking: false }
  const footer = /^BREAKING[ -]CHANGE: /m.test(rest.join('\n'))
  return { subject, type: m[1], breaking: Boolean(m[3]) || footer, bang: Boolean(m[3]), footer }
}

const under = (p, prefixes) => prefixes.some((x) => p.startsWith(x))
const isTest = (p) => /(_test\.go|\.test\.[tj]sx?|\.spec\.[tj]sx?|\.test\.mjs)$/.test(p)

// files: [{ status: 'A'|'M'|'D'|'R', path }]. Returns a list of finding strings.
export function check(message, files, r = rules()) {
  const c = parseCommit(message)
  const out = []
  if (!c.type) return [`not a conventional header: ${c.subject}`]
  const prod = files.filter((f) => !isTest(f.path))
  const visible = prod.some((f) => under(f.path, r.userVisible))
  const removed = files.filter((f) => (f.status === 'D' || f.status === 'R') && under(f.path, r.surface))
  const added = files.filter((f) => f.status === 'A' && !isTest(f.path) && under(f.path, r.newSurface))

  if (c.type === 'feat' && !visible) out.push('feat touches no user-visible path, looks like chore, docs or refactor')
  if (c.type === 'fix' && added.length > 0) out.push(`fix adds new files under ${added[0].path}, looks like feat`)
  if (removed.length > 0 && !c.breaking) out.push(`removes or renames stable surface ${removed[0].path} without ! and BREAKING CHANGE footer`)
  if (c.bang && !c.footer) out.push('uses ! without a BREAKING CHANGE footer')
  if (c.breaking && removed.length === 0 && !prod.some((f) => under(f.path, r.surface))) {
    out.push('declares a breaking change but touches no stable surface file')
  }
  return out
}

const git = (...a) => execFileSync('git', a, { encoding: 'utf8', maxBuffer: 64 << 20 })

export function filesOf(sha) {
  return git('show', '--name-status', '--format=', '-M', sha)
    .split('\n').filter(Boolean).map((l) => {
      const p = l.split('\t')
      return { status: p[0][0], path: p[p.length - 1] }
    })
}

function main() {
  const [base, head = 'HEAD'] = process.argv.slice(2)
  if (!base) {
    process.stderr.write('usage: check-commit-semver.mjs <base-ref> [head-ref]\n')
    process.exit(2)
  }
  const shas = git('rev-list', '--no-merges', '--reverse', `${base}..${head}`).split('\n').filter(Boolean)
  const enforce = process.env.SEMVER_ENFORCE === '1'
  let bad = 0
  for (const sha of shas) {
    const msg = git('log', '-1', '--format=%B', sha)
    if (/^(Merge|Revert) /.test(msg)) continue
    for (const f of check(msg, filesOf(sha))) {
      bad++
      process.stdout.write(`::warning title=commit-semver::${sha.slice(0, 9)} ${f}\n`)
    }
  }
  process.stdout.write(`commit-semver: ${shas.length} commits, ${bad} findings (${enforce ? 'enforcing' : 'report-only'})\n`)
  if (bad > 0 && enforce) process.exit(1)
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main()
