import { describe, expect, it } from 'vitest'
import { defaultTls, formFromCandidate, slug } from './connectDatabaseForm'

describe('connect database form helpers', () => {
  it('prefills an adopted container with its address, network and engine defaults', () => {
    const form = formFromCandidate({
      container_id: 'c1',
      container: 'Coolify_PG.1',
      image: 'postgres:16',
      engine: 'postgres',
      port: 5432,
      suggested_host: 'Coolify_PG.1',
      network: 'coolify',
      suggested_user: 'postgres',
    })
    expect(form.name).toBe('coolify-pg-1')
    expect(form.host).toBe('Coolify_PG.1')
    expect(form.network).toBe('coolify')
    expect(form.port).toBe('5432')
    expect(form.username).toBe('postgres')
    expect(form.tls).toBe('prefer')
    expect(form.container).toBe('Coolify_PG.1')
    expect(form.password).toBe('')
  })

  it('defaults TLS per engine and normalizes names', () => {
    expect(defaultTls('redis')).toBe('disable')
    expect(defaultTls('mysql')).toBe('prefer')
    expect(slug('  My DB!! ')).toBe('my-db')
  })
})
