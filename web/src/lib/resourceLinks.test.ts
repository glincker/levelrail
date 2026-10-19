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

  it('routes buckets and private networks to their pages', () => {
    expect(resourceTarget({ kind: 'bucket', name: 'media', tags: [] }).to).toBe(
      '/settings/storage',
    )
    expect(resourceTarget({ kind: 'network', name: 'core', tags: [] }).to).toBe(
      '/network/private',
    )
  })

  it('routes buckets and private networks to their pages', () => {
    expect(resourceTarget({ kind: 'bucket', name: 'media', tags: [] }).to).toBe(
      '/settings/storage',
    )
    expect(resourceTarget({ kind: 'network', name: 'core', tags: [] }).to).toBe(
      '/network/private',
    )
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
    expect(parseView('{"kind":"widget","tags":[1,"a"]}')).toEqual({
      q: '',
      kind: '',
      tags: ['a'],
    })
  })
})
