import { describe, expect, it } from 'vitest'
import {
  emptyQuery,
  parseView,
  resourceKey,
  resourceTarget,
  serializeView,
} from './resourceLinks'

describe('resourceLinks', () => {
  it('routes nodes by id and falls back to the name', () => {
    expect(
      resourceTarget({ kind: 'node', name: 'vps2', id: 'n1', tags: [] }),
    ).toEqual({ to: '/nodes/$id', params: { id: 'n1' } })
    expect(
      resourceTarget({ kind: 'node', name: 'vps2', tags: [] }).params,
    ).toEqual({ id: 'vps2' })
  })

  it('keys resources by kind and name', () => {
    expect(resourceKey({ kind: 'domain', name: 'a.example.com' })).toBe(
      'domain:a.example.com',
    )
  })

  it('round trips a saved view and rejects junk', () => {
    const q = { q: 'api', kind: 'app' as const, tags: ['team:shop'] }
    expect(parseView(serializeView(q))).toEqual(q)
    expect(parseView('not json')).toEqual(emptyQuery())
    expect(parseView('{"kind":"bucket","tags":[1,"a"]}')).toEqual({
      q: '',
      kind: '',
      tags: ['a'],
    })
  })
})
