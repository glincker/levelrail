import { describe, expect, it } from 'vitest'
import {
  buildProxy,
  inferKind,
  matchPath,
  normalizeTestInput,
  cleanRequestPath,
  MAX_REQUEST_PATH,
  parseProxy,
  statusForKind,
  targetState,
  validateRow,
} from './routesLogic'
import { moveItem } from './rulesDraft'

const row = (from: string, to: string, status = '301') => ({ from, to, status })

describe('kind inference', () => {
  it('derives kind from target and status', () => {
    expect(inferKind('/b', 301)).toBe('redirect')
    expect(inferKind('/index.html', 200)).toBe('rewrite')
    expect(inferKind('app://api/v1', 200)).toBe('proxy')
    expect(inferKind('/gone', 410)).toBe('status')
  })

  it('round trips proxy targets', () => {
    expect(parseProxy(buildProxy('api', '/v1/:splat'))).toEqual({
      app: 'api',
      path: '/v1/:splat',
    })
    expect(parseProxy('/plain')).toBeNull()
  })

  it('picks a sane status when the kind changes', () => {
    expect(statusForKind('proxy', '301')).toBe('200')
    expect(statusForKind('redirect', '200')).toBe('301')
    expect(statusForKind('redirect', '308')).toBe('308')
    expect(statusForKind('status', '301')).toBe('404')
  })

  it('reports the proxy target state', () => {
    expect(targetState('app://api/', 's', ['api'])).toBe('ok')
    expect(targetState('app://nope/', 's', ['api'])).toBe('missing')
    expect(targetState('app://s/', 's', ['s'])).toBe('self')
    expect(targetState('/x', 's', [])).toBeUndefined()
  })
})

describe('validateRow', () => {
  it('rejects a from without a leading slash', () => {
    expect(validateRow(row('old', '/new'), 's', [])).toBe('fromSlash')
  })
  it('rejects braces and backslashes', () => {
    expect(validateRow(row('/a{b}', '/x'), 's', [])).toBe('badChars')
    expect(validateRow(row('/a', '/x\\y'), 's', [])).toBe('badChars')
  })
  it('requires a target unless the status is an error', () => {
    expect(validateRow(row('/a', ''), 's', [])).toBe('toEmpty')
    expect(validateRow(row('/a', '', '410'), 's', [])).toBeNull()
  })
  it('checks the proxy app and path', () => {
    expect(validateRow(row('/a', 'app:///x', '200'), 's', ['api'])).toBe(
      'proxyApp',
    )
    expect(validateRow(row('/a', 'app://zz/x', '200'), 's', ['api'])).toBe(
      'proxyAppMissing',
    )
    expect(validateRow(row('/a', 'app://s/x', '200'), 's', ['s'])).toBe(
      'proxyAppSelf',
    )
    expect(
      validateRow(row('/a', 'app://api/v1', '200'), 's', ['api']),
    ).toBeNull()
    expect(validateRow(row('/a', 'app://api', '200'), 's', ['api'])).toBeNull()
  })
})

describe('matchPath', () => {
  const rules = [
    { from: '/old', to: '/new', status: 301 },
    { from: '/api/*', to: 'app://api/v1/:splat', status: 200 },
    { from: '/*', to: '/index.html', status: 200 },
  ]
  it('first match wins and fills the splat', () => {
    expect(matchPath(rules, '/old')).toMatchObject({
      index: 0,
      kind: 'redirect',
    })
    expect(matchPath(rules, '/api/users/1')).toMatchObject({
      index: 1,
      kind: 'proxy',
      target: 'app://api/v1/users/1',
    })
    expect(matchPath(rules, '/anything')?.index).toBe(2)
  })
  it('returns null without a match and skips invalid rules', () => {
    expect(
      matchPath([{ from: 'bad', to: '/x', status: 301 }], '/bad'),
    ).toBeNull()
  })
})

describe('reorder', () => {
  it('moves an item and ignores out of range targets', () => {
    expect(moveItem(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a'])
    expect(moveItem(['a', 'b'], 0, 5)).toEqual(['a', 'b'])
  })
})

describe('matchPath server semantics', () => {
  it('matches case-insensitively with an optional trailing slash', () => {
    const r = [{ from: '/About', to: '/about-us', status: 301 }]
    expect(matchPath(r, '/about/')?.index).toBe(0)
    expect(matchPath(r, '/ABOUT')?.index).toBe(0)
  })
  it('lets a trailing splat match its bare parent with an empty capture', () => {
    const r = [{ from: '/blog/*', to: '/news/:splat', status: 301 }]
    const m = matchPath(r, '/blog')
    expect(m?.captures).toEqual([{ name: 'splat', value: '' }])
    expect(m?.target).toBe('/news/')
    expect(matchPath(r, '/blog/a/b')?.target).toBe('/news/a/b')
  })
  it('fills named placeholders and escapes the captured value', () => {
    const r = [{ from: '/u/:id', to: '/users?id=:id', status: 302 }]
    expect(matchPath(r, '/u/a b')?.target).toBe('/users?id=a+b')
    expect(matchPath(r, '/u/7')?.captures).toEqual([{ name: 'id', value: '7' }])
    expect(matchPath(r, '/u/7/x')).toBeNull()
  })
  it('carries the query onto redirects only, and not when the target has one', () => {
    const redirect = [{ from: '/a', to: '/b', status: 301 }]
    expect(matchPath(redirect, '/a', 'x=1')?.target).toBe('/b?x=1')
    const own = [{ from: '/a', to: '/b?y=2', status: 301 }]
    expect(matchPath(own, '/a', 'x=1')?.target).toBe('/b?y=2')
    const rewrite = [{ from: '/a', to: '/b', status: 200 }]
    expect(matchPath(rewrite, '/a', 'x=1')?.target).toBe('/b')
  })
  it('skips a redirect that would loop onto itself', () => {
    const r = [
      { from: '/a', to: '/a/', status: 301 },
      { from: '/*', to: '/home', status: 302 },
    ]
    expect(matchPath(r, '/a')?.index).toBe(1)
  })
  it('collapses leading slashes in a redirect location', () => {
    const r = [{ from: '/x/*', to: '/:splat', status: 301 }]
    expect(matchPath(r, '/x//evil.test')?.target).toBe('/evil.test')
  })
  it('reports forced rules and rules that yield to a file', () => {
    const r = [
      { from: '/a', to: '/b', status: 301, force: true },
      { from: '/c', to: '/d', status: 200 },
      { from: '/p/*', to: 'app://api/:splat', status: 200 },
    ]
    expect(matchPath(r, '/a')).toMatchObject({
      force: true,
      yieldsToFile: false,
    })
    expect(matchPath(r, '/c')).toMatchObject({
      kind: 'rewrite',
      yieldsToFile: true,
    })
    expect(matchPath(r, '/p/x')).toMatchObject({
      kind: 'proxy',
      yieldsToFile: false,
    })
  })
  it('reports 404 and 410 rules as status kind', () => {
    const r = [{ from: '/gone', to: '/404.html', status: 410 }]
    expect(matchPath(r, '/gone')).toMatchObject({ kind: 'status', status: 410 })
  })
  it('keeps first-match-wins order and ignores rules the server would reject', () => {
    const r = [
      { from: '/a/*/b', to: '/x', status: 301 },
      { from: '/:splat', to: '/x', status: 301 },
      { from: '/a', to: '/first', status: 301 },
      { from: '/a', to: '/second', status: 301 },
    ]
    expect(matchPath(r, '/a')?.index).toBe(2)
  })
  it('bounds the path like the server', () => {
    const r = [{ from: '/*', to: '/i', status: 200 }]
    expect(matchPath(r, '/' + 'a'.repeat(MAX_REQUEST_PATH))).toBeNull()
  })
})

describe('test input', () => {
  it('adds a leading slash and splits the query off', () => {
    expect(normalizeTestInput('about', '')).toEqual({
      path: '/about',
      query: '',
    })
    expect(normalizeTestInput('/a?x=1#top', '')).toEqual({
      path: '/a',
      query: 'x=1',
    })
    expect(normalizeTestInput('/a', '?y=2')).toEqual({
      path: '/a',
      query: 'y=2',
    })
  })
  it('treats empty input as idle', () => {
    expect(normalizeTestInput('  ', '').path).toBe('')
  })
  it('cleans dot segments and keeps one trailing slash', () => {
    expect(cleanRequestPath('/a/./b/../c/')).toBe('/a/c/')
    expect(cleanRequestPath('//a//b')).toBe('/a/b')
    expect(cleanRequestPath('/')).toBe('/')
  })
})
