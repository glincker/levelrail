import { describe, expect, it } from 'vitest'
import { rankItems, relatedFor, scoreToken } from './paletteSearch'

const items = [
  { key: 'nav-dashboard', label: 'Dashboard' },
  { key: 'nav-usage', label: 'Usage and cost' },
  { key: 'settings-registry-credentials', label: 'Registry credentials' },
  { key: 'nav-backups', label: 'Backups' },
  { key: 'nav-databases', label: 'Databases' },
  { key: 'action-create-database', label: 'Create database' },
  { key: 'nav-help', label: 'Help' },
]

describe('scoreToken', () => {
  it('ranks prefix above word start above substring above subsequence', () => {
    const prefix = scoreToken('back', 'Backups') ?? 0
    const word = scoreToken('back', 'Go to backups') ?? 0
    const sub = scoreToken('ack', 'Backups') ?? 0
    const seq = scoreToken('bks', 'Backups') ?? 0
    expect(prefix).toBeGreaterThan(word)
    expect(word).toBeGreaterThan(sub)
    expect(sub).toBeGreaterThan(seq)
    expect(scoreToken('zzz', 'Backups')).toBeNull()
  })
})

describe('rankItems', () => {
  it('surfaces related pages for container, not only title matches', () => {
    const keys = rankItems(items, 'container').map((i) => i.key)
    expect(keys[0]).toBe('settings-registry-credentials')
    expect(keys).not.toContain('nav-help')
    expect(keys).not.toContain('nav-dashboard')
  })

  it('puts Databases first for db and keeps related backups', () => {
    const keys = rankItems(items, 'db').map((i) => i.key)
    expect(keys[0]).toBe('nav-databases')
    expect(keys).toContain('nav-backups')
    expect(keys).toContain('action-create-database')
  })

  it('maps domain words to domain pages', () => {
    const list = [
      { key: 'nav-domains', label: 'Domains' },
      { key: 'nav-dns', label: 'DNS zones' },
      { key: 'nav-help', label: 'Help' },
    ]
    const keys = rankItems(list, 'ssl').map((i) => i.key)
    expect(keys).toEqual(['nav-domains'])
  })

  it('requires every word to match and keeps order for an empty query', () => {
    expect(rankItems(items, 'create zzz')).toEqual([])
    expect(rankItems(items, '  ')).toEqual(items)
  })
})

describe('relatedFor', () => {
  it('suggests registry, builds and a Portainer template for container', () => {
    const r = relatedFor('container')
    expect(r.keys[0]).toBe('settings-registry-credentials')
    expect(r.templates).toContain('portainer')
  })

  it('suggests databases and backups for db', () => {
    expect(relatedFor('db').keys.slice(0, 2)).toEqual([
      'nav-databases',
      'nav-backups',
    ])
  })

  it('stays quiet for unrelated text and one-letter input', () => {
    expect(relatedFor('zebra').keys).toEqual([])
    expect(relatedFor('d').keys).toEqual([])
  })
})
