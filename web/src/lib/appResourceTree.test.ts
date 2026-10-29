import { describe, expect, it } from 'vitest'
import { buildAppResourceTree } from './appResourceTree'

const app = {
  name: 'web',
  image: 'ghcr.io/x/web:1.2@sha256:abcdef0123456789',
  replicas: 2,
  domains: ['b.example.com', 'a.example.com'],
  volumes: [{ name: 'data', container_path: '/data' }],
  database_env: { DATABASE_URL: { database: 'pg', field: 'url' } },
  env: { API_KEY: 'super-secret-value', MODE: 'prod' },
  secret_env: ['TOKEN', 'MODE'],
}

describe('buildAppResourceTree', () => {
  it('nests groups in order and counts distinct env keys', () => {
    const t = buildAppResourceTree(app, [
      {
        Type: 'Ready',
        Status: 'True',
        Reason: 'Up',
        Message: '',
        LastTransitionTime: '',
      },
    ])
    expect(t.groups.map((g) => g.id)).toEqual([
      'deployment',
      'replicas',
      'domains',
      'volumes',
      'databases',
      'env',
    ])
    const by = (id: string) => {
      const g = t.groups.find((x) => x.id === id)
      if (!g) throw new Error(id)
      return g
    }
    expect(by('deployment').leaves[0]?.digest).toBe('sha256:abcdef0123456789')
    expect(by('replicas').count).toBe(2)
    expect(by('replicas').leaves[0]?.detail).toBe('True (Up)')
    expect(by('domains').leaves.map((l) => l.label)).toEqual([
      'a.example.com',
      'b.example.com',
    ])
    expect(by('env').count).toBe(3)
  })

  it('never includes env values', () => {
    const json = JSON.stringify(buildAppResourceTree(app))
    expect(json).not.toContain('super-secret-value')
    expect(json).not.toContain('prod')
  })

  it('omits empty groups', () => {
    const t = buildAppResourceTree({ name: 'x', image: '', replicas: 0 })
    expect(t.groups.map((g) => g.id)).toEqual(['replicas'])
  })
})
