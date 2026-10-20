import { describe, expect, it } from 'vitest'
import {
  buildLogQuery,
  formatFieldFilter,
  parseFieldFilter,
  parseFieldFilters,
} from './logFilters'

describe('parseFieldFilter', () => {
  it.each([
    ['status=500', { key: 'status', op: '=', value: '500' }],
    ['status!=200', { key: 'status', op: '!=', value: '200' }],
    ['req.path~/api', { key: 'req.path', op: '~', value: '/api' }],
    ['a=b=c', { key: 'a', op: '=', value: 'b=c' }],
    ['  user_id=7 ', { key: 'user_id', op: '=', value: '7' }],
  ])('parses %s', (raw, want) => {
    expect(parseFieldFilter(raw)).toEqual(want)
  })

  it.each(['', 'novalue', '=x', 'key=', 'bad key=1'])('rejects %j', (raw) => {
    expect(parseFieldFilter(raw)).toBeNull()
  })

  it('round trips through format', () => {
    const f = { key: 'a.b', op: '!=' as const, value: 'x y' }
    expect(parseFieldFilter(formatFieldFilter(f))).toEqual(f)
  })

  it('drops unparsable entries from a list', () => {
    expect(
      parseFieldFilters(['a=1', 'nope', 'b~2']).map(formatFieldFilter),
    ).toEqual(['a=1', 'b~2'])
  })
})

describe('buildLogQuery', () => {
  it('writes one field param per chip and omits empty filters', () => {
    const params = buildLogQuery({
      from: new Date('2026-10-10T00:00:00Z'),
      to: new Date('2026-10-10T01:00:00Z'),
      q: 'timeout',
      container: 'abc',
      stream: 'stderr',
      level: 'warn',
      fields: [
        { key: 'status', op: '=', value: '500' },
        { key: 'route', op: '~', value: '/pay' },
      ],
      limit: 1000,
    })
    expect(params.get('from')).toBe('2026-10-10T00:00:00.000Z')
    expect(params.get('q')).toBe('timeout')
    expect(params.get('container')).toBe('abc')
    expect(params.get('stream')).toBe('stderr')
    expect(params.get('level')).toBe('warn')
    expect(params.getAll('field')).toEqual(['status=500', 'route~/pay'])
    expect(params.get('limit')).toBe('1000')
    expect(buildLogQuery({}).toString()).toBe('')
  })
})
