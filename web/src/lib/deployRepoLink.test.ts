import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { isValidAppName, isValidPort } from './deployLink'
import {
  buildRepoLinkUrl,
  isValidBranch,
  isValidRepoUrl,
  isValidTemplateId,
  parseRepoLink,
  repoLinkHtml,
  repoLinkMarkdown,
} from './deployRepoLink'

interface CaseSet {
  valid: string[]
  invalid: string[]
}
const shared = JSON.parse(
  readFileSync(
    path.resolve(
      import.meta.dirname,
      '../../../internal/deploylink/testdata/cases.json',
    ),
    'utf8',
  ),
) as Record<string, CaseSet>

const validators: Record<string, (v: string) => boolean> = {
  repo: isValidRepoUrl,
  branch: isValidBranch,
  name: isValidAppName,
  template: isValidTemplateId,
  port: isValidPort,
}

describe('shared validation cases (same file as the Go tests)', () => {
  for (const [field, fn] of Object.entries(validators)) {
    const set = shared[field]
    it(`${field}: valid and invalid inputs`, () => {
      expect(set?.valid.length).toBeGreaterThan(0)
      expect(set?.invalid.length).toBeGreaterThan(0)
      for (const v of set?.valid ?? []) expect(fn(v), v).toBe(true)
      for (const v of set?.invalid ?? []) expect(fn(v), v).toBe(false)
    })
  }
})

describe('parseRepoLink', () => {
  it('accepts a full valid link', () => {
    const p = parseRepoLink(
      {
        repo: 'https://github.com/you/app.git',
        branch: 'main',
        name: 'app',
        port: '3000',
      },
      new Set(['postgres']),
    )
    expect(p).toMatchObject({
      hasParams: true,
      repo: 'https://github.com/you/app.git',
      branch: 'main',
      name: 'app',
      port: 3000,
      ignored: [],
    })
  })

  it('drops credentials, bad schemes and env values, and names them', () => {
    const p = parseRepoLink({
      repo: 'https://user:pw@github.com/you/app',
      env: 'TOKEN=abc',
      secret: 'x',
      port: '99999',
    })
    expect(p.repo).toBeUndefined()
    expect(p.port).toBeUndefined()
    expect(p.ignored).toEqual(['repo', 'port', 'env', 'secret'])
  })

  it('rejects unknown templates once the catalog is loaded', () => {
    const known = new Set(['postgres'])
    expect(parseRepoLink({ template: 'postgres' }, known).template).toBe(
      'postgres',
    )
    const bad = parseRepoLink({ template: 'nope' }, known)
    expect(bad.template).toBeUndefined()
    expect(bad.ignored).toEqual(['template'])
  })

  it('keeps repo over template and ignores branch without a repo', () => {
    const both = parseRepoLink({
      repo: 'https://github.com/you/app',
      template: 'postgres',
    })
    expect(both.repo).toBeDefined()
    expect(both.template).toBeUndefined()
    expect(
      parseRepoLink({ template: 'postgres', branch: 'main' }).ignored,
    ).toEqual(['branch'])
  })

  it('reports no params for an empty query', () => {
    expect(parseRepoLink({}).hasParams).toBe(false)
  })
})

describe('building links', () => {
  it('matches the Go link format', () => {
    const r = buildRepoLinkUrl('https://lr.example.com/', {
      repo: 'https://github.com/you/app.git',
      branch: 'main',
      name: 'app',
      port: '3000',
    })
    expect(r.url).toBe(
      'https://lr.example.com/deploy?repo=https%3A%2F%2Fgithub.com%2Fyou%2Fapp.git&branch=main&name=app&port=3000',
    )
  })

  it('flags invalid fields and returns no url', () => {
    const r = buildRepoLinkUrl('https://lr.example.com', {
      repo: 'file:///etc/passwd',
      port: '0',
    })
    expect(r.url).toBeNull()
    expect(r.invalid).toEqual(['repo', 'port'])
  })

  it('escapes snippets', () => {
    const link = 'https://lr.example.com/deploy?repo=a&branch=b'
    expect(repoLinkMarkdown('Deploy', link)).toContain(`](${link})`)
    expect(repoLinkHtml('a"b', link)).toContain('&amp;branch=b')
    expect(repoLinkHtml('a"b', link)).toContain('alt="a&quot;b"')
  })
})
