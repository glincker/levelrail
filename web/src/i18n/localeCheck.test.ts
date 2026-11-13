import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterAll, describe, expect, it } from 'vitest'
import { checkLanguage, requiredPluralCategories } from './localeCheck.mjs'

const EM = String.fromCharCode(0x2014)

describe('requiredPluralCategories', () => {
  it.each([
    ['en', ['one', 'other']],
    ['es', ['one', 'other']],
    ['fr', ['one', 'other']],
    ['pt-BR', ['one', 'other']],
    ['de', ['one', 'other']],
    ['ja', ['other']],
    ['zh-CN', ['other']],
    ['ru', ['few', 'many', 'one', 'other']],
  ])('%s', (lang, expected) => {
    expect(requiredPluralCategories(lang)).toEqual(expected)
  })
})

describe('checkLanguage fixtures', () => {
  const root = mkdtempSync(join(tmpdir(), 'locales-'))
  afterAll(() => rmSync(root, { recursive: true, force: true }))

  function write(lang: string, data: object) {
    mkdirSync(join(root, lang), { recursive: true })
    writeFileSync(join(root, lang, 'common.json'), JSON.stringify(data))
  }
  write('en', {
    apps_one: '{{count}} app on Docker',
    apps_other: '{{count}} apps on Docker',
    title: 'Hello',
  })
  const none = { missing: [], knownIssues: [] }

  it('passes a complete language', () => {
    write('ja', { apps_other: '{{count}} Docker', title: 'Hi' })
    const r = checkLanguage('ja', { localesDir: root, baseline: none })
    expect(r.problems).toEqual([])
    expect(r.unlisted).toEqual([])
  })

  it('flags a missing plural form for the language', () => {
    write('ru', {
      apps_one: '{{count}} Docker',
      apps_other: '{{count}} Docker',
      title: 'Hi',
    })
    const r = checkLanguage('ru', { localesDir: root, baseline: none })
    expect(r.unlisted).toEqual(['common:apps_few', 'common:apps_many'])
  })

  it('flags dashes, lost placeholders and lost glossary terms', () => {
    write('de', {
      apps_one: `{{count}} App ${EM} Docker`,
      apps_other: 'Apps auf Docker',
      title: 'Hallo',
    })
    const r = checkLanguage('de', { localesDir: root, baseline: none })
    const all = r.problems.join('\n')
    expect(all).toMatch(/apps_one: contains an em or en dash/)
    expect(all).toMatch(/apps_other: placeholders or tags differ/)
  })

  it('flags a dropped protected term', () => {
    write('it', {
      apps_one: '{{count}} app',
      apps_other: '{{count}} app',
      title: 'Ciao',
    })
    const r = checkLanguage('it', { localesDir: root, baseline: none })
    expect(r.problems.join('\n')).toMatch(/lost protected term "Docker"/)
  })

  it('accepts baseline entries and reports stale ones', () => {
    write('fr', {
      apps_one: '{{count}} app Docker',
      apps_other: '{{count}} apps Docker',
    })
    const r = checkLanguage('fr', {
      localesDir: root,
      baseline: { missing: ['common:title', 'common:gone'], knownIssues: [] },
    })
    expect(r.unlisted).toEqual([])
    expect(r.stale).toEqual(['common:gone'])
  })
})
