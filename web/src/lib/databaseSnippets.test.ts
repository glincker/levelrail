import { describe, expect, it } from 'vitest'
import { buildSnippets, MASK } from './databaseSnippets'

const base = {
  host: 'db',
  port: 5432,
  user: 'postgres',
  database: 'postgres',
  tls: false,
  revealPlaceholder: false,
}

describe('buildSnippets', () => {
  it('masks the password by default', () => {
    const s = buildSnippets({ ...base, engine: 'postgres' })
    expect(s.uri).toContain(`:${MASK}@`)
    expect(s.env.startsWith('DATABASE_URL=postgresql://')).toBe(true)
    expect(s.node).toContain('pg')
  })
  it('shows the placeholder when asked and requires TLS when enabled', () => {
    const s = buildSnippets({
      ...base,
      engine: 'postgres',
      tls: true,
      revealPlaceholder: true,
    })
    expect(s.uri).toContain('<password>')
    expect(s.uri).toContain('sslmode=require')
  })
  it('is engine aware for redis', () => {
    const s = buildSnippets({ ...base, engine: 'redis', port: 6379, tls: true })
    expect(s.uri.startsWith('rediss://')).toBe(true)
    expect(s.env.startsWith('REDIS_URL=')).toBe(true)
    expect(s.cli).toContain('--tls')
    expect(s.go).toContain('redis.ParseURL')
  })
})
