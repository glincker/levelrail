import { describe, expect, it } from 'vitest'
import {
  buildConnectSnippets,
  connectDatabase,
  connectHasPassword,
  connectUser,
  connectFrameworks,
  downloadContent,
  type ConnectInput,
} from './connectSnippets'

const base: ConnectInput = {
  engine: 'postgres',
  host: '127.0.0.1',
  port: 5432,
  database: 'app',
  user: 'postgres',
  password: null,
  tls: false,
}

describe('buildConnectSnippets', () => {
  it('postgres covers every framework with JDBC for Spring', () => {
    const s = buildConnectSnippets(base)
    expect(s.springProperties).toContain(
      'spring.datasource.url=jdbc:postgresql://127.0.0.1:5432/app',
    )
    expect(s.springProperties).toContain('password=<password>')
    expect(s.springYaml).toContain(
      '    url: "jdbc:postgresql://127.0.0.1:5432/app"',
    )
    expect(s.node).toContain("from 'pg'")
    expect(s.python).toContain('psycopg')
    expect(s.go).toContain('pgx.Connect')
    expect(s.dotnet).toContain('Host=127.0.0.1;Port=5432;Database=app')
    expect(s.url).toBe(
      'postgresql://postgres:<password>@127.0.0.1:5432/app?sslmode=prefer',
    )
    expect(s.env).toContain('DATABASE_URL=postgresql://')
  })

  it('mysql and mariadb use their own JDBC prefix and DSN', () => {
    const my = buildConnectSnippets({ ...base, engine: 'mysql', port: 3306 })
    const ma = buildConnectSnippets({ ...base, engine: 'mariadb', port: 3306 })
    expect(my.springProperties).toContain('jdbc:mysql://127.0.0.1:3306/app')
    expect(ma.springProperties).toContain('jdbc:mariadb://127.0.0.1:3306/app')
    expect(my.node).toContain('mysql2')
    expect(my.python).toContain('pymysql')
    expect(my.go).toContain('tcp(127.0.0.1:3306)/app')
    expect(my.dotnet).toContain('Server=127.0.0.1;Port=3306')
  })

  it('mongodb uses the spring mongodb uri with directConnection', () => {
    const s = buildConnectSnippets({
      ...base,
      engine: 'mongodb',
      port: 27017,
      database: 'admin',
      user: 'levelrail',
    })
    expect(s.springProperties).toBe(
      'spring.data.mongodb.uri=mongodb://levelrail:<password>@127.0.0.1:27017/admin?authSource=admin&directConnection=true',
    )
    expect(s.springYaml).toContain('      uri: "mongodb://')
    expect(s.node).toContain('mongoose')
    expect(s.python).toContain('MongoClient')
    expect(s.go).toContain('mongo.Connect')
    expect(s.env).toContain('MONGODB_URI=')
  })

  it.each(['redis', 'valkey', 'keydb', 'dragonfly'])(
    '%s uses spring.data.redis keys and the redis clients',
    (engine) => {
      const s = buildConnectSnippets({
        ...base,
        engine,
        port: 6379,
        database: '',
        user: '',
        tls: true,
      })
      expect(s.springProperties).toContain('spring.data.redis.host=127.0.0.1')
      expect(s.springProperties).toContain('spring.data.redis.ssl.enabled=true')
      expect(s.springYaml).toContain('    redis:')
      expect(s.node).toContain('ioredis')
      expect(s.python).toContain('redis.from_url')
      expect(s.go).toContain('redis.ParseURL')
      expect(s.url).toBe('rediss://127.0.0.1:6379')
      expect(s.env).toContain('REDIS_URL=rediss://')
    },
  )

  it('other engines only get url and env', () => {
    const s = buildConnectSnippets({ ...base, engine: 'qdrant', port: 6333 })
    expect(Object.keys(s).sort()).toEqual(['env', 'url'])
    expect(connectFrameworks('qdrant')).toEqual(['url', 'env'])
  })

  it('percent-encodes a revealed password in URLs only', () => {
    const s = buildConnectSnippets({ ...base, password: 'p@ss/w:rd' })
    expect(s.url).toContain(':p%40ss%2Fw%3Ard@')
    expect(s.springProperties).toContain('password=p@ss/w:rd')
  })

  it('requires tls where the engine supports it', () => {
    const s = buildConnectSnippets({ ...base, tls: true })
    expect(s.url).toContain('sslmode=require')
    expect(s.springProperties).toContain('?sslmode=require')
  })
})

describe('downloadContent', () => {
  it('returns files with a trailing newline and null for unsupported engines', () => {
    expect(downloadContent('env', base)).toContain('DATABASE_URL=')
    expect(downloadContent('properties', base)).toContain('spring.datasource')
    expect(downloadContent('yaml', base)).toContain('spring:')
    expect(downloadContent('yaml', { ...base, engine: 'qdrant' })).toBeNull()
  })
})

describe('engine defaults', () => {
  it.each([
    ['postgres', 'demo', 'demo'],
    ['mysql', 'demo', 'demo'],
    ['mariadb', 'demo', 'demo'],
    ['mongodb', 'demo', 'admin'],
  ])('%s has a generated password', (engine, user, database) => {
    expect(connectHasPassword(engine)).toBe(true)
    expect(connectUser(engine, 'demo')).toBe(user)
    expect(connectDatabase(engine, 'demo')).toBe(database)
  })

  it.each(['redis', 'valkey', 'keydb', 'dragonfly'])(
    '%s is passwordless',
    (engine) => {
      expect(connectHasPassword(engine)).toBe(false)
      expect(connectUser(engine, 'demo')).toBe('')
      expect(connectDatabase(engine, 'demo')).toBe('')
    },
  )

  it('redis family has no password anywhere', () => {
    const s = buildConnectSnippets({
      ...base,
      engine: 'valkey',
      port: 6379,
      database: '',
      user: '',
    })
    expect(JSON.stringify(s)).not.toContain('password')
  })
})
