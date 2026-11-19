import { describe, expect, it } from 'vitest'
import {
  buildProxy,
  inferKind,
  matchPath,
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
