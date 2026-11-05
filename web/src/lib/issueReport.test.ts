import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  COMPONENT_OPTIONS,
  TEMPLATE_FIELDS,
  browserInfo,
  buildDraft,
  buildIssueUrl,
  componentForRoute,
  issueUrlMaxChars,
  pickTemplate,
  redactText,
  routeTemplate,
  type IssueDraft,
} from './issueReport'

// Assembled at runtime so secret scanners do not flag the fixture.
const PLANTED = ['sk', 'live', 'abcdefghijklmnop1234567890'].join('_')
const SUPPORT = 'https://github.com/acme/repo/issues'

function ymlOptions(file: string, id: string): string[] {
  const text = readFileSync(
    resolve(__dirname, '../../../.github/ISSUE_TEMPLATE', file),
    'utf8',
  )
  const block = text.split(`id: ${id}`)[1] ?? ''
  const after = block.split('options:')[1] ?? ''
  const out: string[] = []
  for (const line of after.split('\n').slice(1)) {
    const m = /^\s+- (.+)$/.exec(line)
    if (!m) break
    out.push(m[1] ?? '')
  }
  return out
}

describe('issue template drift', () => {
  it('component options match bug_report.yml', () => {
    expect(Object.values(COMPONENT_OPTIONS)).toEqual(
      ymlOptions('bug_report.yml', 'component'),
    )
  })
  it('every field we send exists in its template', () => {
    for (const [tpl, specs] of Object.entries(TEMPLATE_FIELDS)) {
      const text = readFileSync(
        resolve(__dirname, '../../../.github/ISSUE_TEMPLATE', `${tpl}.yml`),
        'utf8',
      )
      for (const s of specs) expect(text).toContain(`id: ${s.id}`)
    }
  })
})

describe('buildIssueUrl', () => {
  const draft: IssueDraft = {
    template: 'bug_report',
    title: 'bug: a & b',
    fields: {
      version: 'v1.2.3',
      actual: 'x'.repeat(5000),
      logs: 'é😀'.repeat(900),
    },
  }
  it('encodes and selects the template', () => {
    const u = buildIssueUrl(draft, SUPPORT + '/', 100000)
    expect(u.startsWith(`${SUPPORT}/new?template=bug_report.yml`)).toBe(true)
    expect(u).toContain('title=bug%3A%20a%20%26%20b')
  })
  it('truncates under the limit without breaking encoding', () => {
    const u = buildIssueUrl(draft, SUPPORT, 1500)
    expect(u.length).toBeLessThanOrEqual(1500)
    expect(() => decodeURIComponent(u)).not.toThrow()
    expect(decodeURIComponent(u)).toContain('[...truncated]')
  })
  it('drops fields when truncation is not enough', () => {
    const u = buildIssueUrl(
      { ...draft, fields: { version: 'v1', context: 'c'.repeat(50) } },
      SUPPORT,
      130,
    )
    expect(u.length).toBeLessThanOrEqual(130)
    expect(u).toContain('template=bug_report.yml')
  })
  it('returns the url unchanged when it is not an issues url', () => {
    expect(buildIssueUrl(draft, 'https://example.test/support', 100)).toBe(
      'https://example.test/support',
    )
  })
  it('reads the limit from env with a safe default', () => {
    expect(issueUrlMaxChars('3000')).toBe(3000)
    expect(issueUrlMaxChars('nope')).toBe(6000)
  })
})

describe('pickTemplate and drafts', () => {
  it('maps contexts to templates', () => {
    expect(pickTemplate({ source: 'error', pathname: '/apps' })).toBe(
      'bug_report',
    )
    expect(pickTemplate({ source: 'feature', pathname: '/' })).toBe(
      'feature_request',
    )
    expect(pickTemplate({ source: 'help', pathname: '/help/ssl' })).toBe(
      'docs_problem',
    )
    expect(pickTemplate({ source: 'help', pathname: '/templates' })).toBe(
      'template_request',
    )
    expect(pickTemplate({ source: 'help', pathname: '/import/compose' })).toBe(
      'migration_problem',
    )
    expect(pickTemplate({ source: 'help', pathname: '/apps' })).toBe(
      'bug_report',
    )
  })
  it('guesses the component from the route', () => {
    expect(componentForRoute('/nodes/$id')).toBe('Node agent')
    expect(componentForRoute('/domains')).toBe('Ingress / TLS (Caddy)')
    expect(componentForRoute('/settings')).toBe('Frontend / dashboard')
  })
  it('strips layout segments and query from routes', () => {
    expect(routeTemplate('/_authed/apps/$name/?token=abc')).toBe('/apps/$name')
  })
  it('error draft carries message and request id, redacted', () => {
    const d = buildDraft({
      source: 'error',
      pathname: '/apps/x',
      route: '/apps/$name',
      version: 'v0.9.1',
      browser: browserInfo({
        userAgent: 'Mozilla/5.0 (Macintosh) Chrome/120.0 Safari/537.36',
        platform: 'MacIntel',
      }),
      errorMessage: `failed token=${PLANTED}`,
      requestId: 'req-1',
    })
    expect(d.fields.actual).toContain('Request ID: req-1')
    expect(d.fields.actual).not.toContain(PLANTED)
    expect(d.fields.context).toContain('Chrome 120 on macOS')
    expect(d.fields.context).toContain('/apps/$name')
  })
})

describe('redactText', () => {
  it('removes planted secrets, emails and query strings', () => {
    const out = redactText(
      'Authorization: Bearer abc.def.ghi1234 password=hunter2 me@example.com ' +
        'https://u:pw@host.test/p?x=1 ' +
        'A1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6Q7r8',
    )
    for (const bad of [
      'hunter2',
      'me@example.com',
      'u:pw',
      'x=1',
      'A1b2C3d4E5',
      'abc.def',
    ]) {
      expect(out).not.toContain(bad)
    }
  })
})
