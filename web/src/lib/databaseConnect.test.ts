import { describe, expect, it } from 'vitest'
import { connectStrings } from './databaseConnect'

describe('connectStrings', () => {
  it('builds a postgres uri with a password placeholder', () => {
    const s = connectStrings({
      engine: 'postgres',
      host: '127.0.0.1',
      port: 5433,
      user: 'app',
      database: 'main',
      tls: true,
    })
    expect(s.uri).toBe(
      'postgresql://app:<password>@127.0.0.1:5433/main?sslmode=require',
    )
    expect(s.cli).toContain('psql -h 127.0.0.1')
  })

  it('uses rediss when TLS is on', () => {
    const s = connectStrings({
      engine: 'redis',
      host: 'h',
      port: 6379,
      user: '',
      database: '',
      tls: true,
    })
    expect(s.uri.startsWith('rediss://')).toBe(true)
    expect(s.cli).toContain('--tls')
  })
})
