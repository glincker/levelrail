import { describe, expect, it } from 'vitest'
import {
  buildDeployButtonUrl,
  deployButtonMarkdown,
  isValidAppName,
  isValidEnvKey,
  isValidHealthPath,
  isValidImageRef,
  isValidPort,
  parseDeployLink,
  sourceHostOf,
  stringifySearch,
} from './deployLink'

describe('deploy link validators', () => {
  it.each([
    ['nginx', true],
    ['nginx:1.27', true],
    ['ghcr.io/you/app:latest', true],
    ['localhost:5000/team/app:v1', true],
    [`ghcr.io/a/b@sha256:${'a'.repeat(64)}`, true],
    ['Nginx', false],
    ['', false],
    ['app:', false],
    ['a b', false],
    ['http://evil/app', false],
    ['app;rm -rf', false],
    ['-app', false],
  ])('image %j -> %s', (v, want) => {
    expect(isValidImageRef(v)).toBe(want)
  })

  it.each([
    ['my-app', true],
    ['a', true],
    ['My-App', false],
    ['-a', false],
    ['a-', false],
    ['a'.repeat(64), false],
    ['a_b', false],
  ])('name %j -> %s', (v, want) => {
    expect(isValidAppName(v)).toBe(want)
  })

  it.each([
    ['1', true],
    ['65535', true],
    ['0', false],
    ['65536', false],
    ['-1', false],
    ['80.5', false],
    ['abc', false],
    ['', false],
  ])('port %j -> %s', (v, want) => {
    expect(isValidPort(v)).toBe(want)
  })

  it.each([
    ['DATABASE_URL', true],
    ['_X', true],
    ['api_key', false],
    ['1KEY', false],
    ['A-B', false],
    ['KEY=value', false],
  ])('env key %j -> %s', (v, want) => {
    expect(isValidEnvKey(v)).toBe(want)
  })

  it.each([
    ['/healthz', true],
    ['/', true],
    ['healthz', false],
    ['//evil.example', false],
    ['/a b', false],
    ['/a\\b', false],
  ])('health %j -> %s', (v, want) => {
    expect(isValidHealthPath(v)).toBe(want)
  })

  it.each([
    ['https://github.com/a/b', 'github.com'],
    ['http://example.com:8080/x', 'example.com:8080'],
    ['https://user:pw@example.com/', null],
    ['javascript:alert(1)', null],
    ['ftp://example.com', null],
    ['nonsense', null],
  ])('source %j -> %s', (v, want) => {
    expect(sourceHostOf(v)).toBe(want)
  })
})

describe('parseDeployLink', () => {
  it('is inactive without deploy=1', () => {
    const p = parseDeployLink({ image: 'nginx' })
    expect(p.active).toBe(false)
    expect(p.image).toBeUndefined()
  })

  it('accepts a full valid link', () => {
    const p = parseDeployLink({
      deploy: '1',
      image: 'ghcr.io/you/app:1',
      name: 'my-app',
      port: '3000',
      env: 'A_KEY,B_KEY',
      health: '/healthz',
      source: 'https://github.com/you/app',
    })
    expect(p).toMatchObject({
      active: true,
      image: 'ghcr.io/you/app:1',
      name: 'my-app',
      port: 3000,
      env: ['A_KEY', 'B_KEY'],
      health: '/healthz',
      sourceHost: 'github.com',
      ignored: [],
    })
  })

  it('ignores invalid params and names them', () => {
    const p = parseDeployLink({
      deploy: '1',
      image: 'Bad Image',
      name: 'Bad_Name',
      port: '99999',
      env: 'GOOD,bad',
      health: 'nope',
      source: 'javascript:alert(1)',
    })
    expect(p.image).toBeUndefined()
    expect(p.name).toBeUndefined()
    expect(p.port).toBeUndefined()
    expect(p.env).toEqual(['GOOD'])
    expect(p.health).toBeUndefined()
    expect(p.ignored).toEqual([
      'image',
      'name',
      'port',
      'env',
      'health',
      'source',
    ])
  })

  it('never accepts env values and reports unknown params', () => {
    const p = parseDeployLink({ deploy: '1', image: 'nginx', SECRET: 'x' })
    expect(p.env).toEqual([])
    expect(p.ignored).toEqual(['SECRET'])
  })

  it('derives a name from the image when none is given', () => {
    expect(
      parseDeployLink({ deploy: '1', image: 'ghcr.io/a/site:v2' }).name,
    ).toBe('site')
  })
})

describe('stringifySearch', () => {
  it('coerces router-parsed numbers and drops objects', () => {
    expect(
      stringifySearch({ port: 3000, deploy: 1, x: { a: 1 }, name: 'a' }),
    ).toEqual({ port: '3000', deploy: '1', name: 'a' })
  })
})

describe('parseDeployLink hostile input', () => {
  it.each([
    ['env value pair', { env: 'TOKEN=abc123' }, 'env', []],
    ['env lowercase', { env: 'token' }, 'env', []],
    ['env mixed', { env: 'GOOD,bad=1' }, 'env', ['GOOD']],
    [
      'oversized env list',
      { env: Array.from({ length: 51 }, (_, i) => `K${i}`).join(',') },
      'env',
      null,
    ],
    ['oversized image', { image: 'a'.repeat(256) }, 'image', []],
    [
      'script source',
      { image: 'nginx', source: 'javascript:alert(1)' },
      'source',
      [],
    ],
    [
      'userinfo source',
      { image: 'nginx', source: 'https://u:p@github.com/a/b' },
      'source',
      [],
    ],
    [
      'oversized source',
      { image: 'nginx', source: `https://example.com/${'a'.repeat(2100)}` },
      'source',
      [],
    ],
    [
      'protocol-relative health',
      { image: 'nginx', health: '//evil.example' },
      'health',
      [],
    ],
  ])('%s', (_label, extra, rejected, env) => {
    const got = parseDeployLink({ deploy: '1', image: 'nginx', ...extra })
    expect(got.ignored).toContain(rejected)
    if (env !== null) expect(got.env).toEqual(env)
    else expect(got.env.length).toBeLessThanOrEqual(50)
  })

  it('never exposes a secret-looking value anywhere in the result', () => {
    const got = parseDeployLink({
      deploy: '1',
      image: 'nginx',
      env: 'API_KEY=sk_live_123',
      password: 'hunter2',
    })
    expect(JSON.stringify(got)).not.toContain('sk_live_123')
    expect(JSON.stringify(got)).not.toContain('hunter2')
  })
})

describe('buildDeployButtonUrl', () => {
  it('builds a url that parseDeployLink accepts unchanged', () => {
    const { url, invalid } = buildDeployButtonUrl({
      image: 'ghcr.io/you/app:1.2',
      name: 'my-app',
      port: '3000',
      env: 'DATABASE_URL\nSESSION_KEY, DATABASE_URL',
      health: '/healthz',
      source: 'https://github.com/you/app',
    })
    expect(invalid).toEqual([])
    const params = Object.fromEntries(new URL(url ?? '').searchParams)
    const parsed = parseDeployLink({ deploy: '1', ...params })
    expect(parsed.ignored).toEqual([])
    expect(parsed.env).toEqual(['DATABASE_URL', 'SESSION_KEY'])
    expect(parsed.port).toBe(3000)
  })

  it.each([
    ['bad image', { image: 'Not An Image' }, ['image']],
    ['env with value', { image: 'nginx', env: 'A=b' }, ['env']],
    ['bad port', { image: 'nginx', port: '70000' }, ['port']],
    ['bad health', { image: 'nginx', health: 'healthz' }, ['health']],
    ['bad source', { image: 'nginx', source: 'ftp://x.io/a' }, ['source']],
    ['bad name', { image: 'nginx', name: 'Bad_Name' }, ['name']],
  ])('%s', (_label, input, invalid) => {
    const got = buildDeployButtonUrl(input)
    expect(got.url).toBeNull()
    expect(got.invalid).toEqual(invalid)
  })

  it('wraps the url in a markdown badge', () => {
    const md = deployButtonMarkdown('https://levelrail.com/deploy?image=nginx')
    expect(md).toBe(
      '[![Deploy to Levelrail](https://cdn.jsdelivr.net/gh/glincker/levelrail@main/docs/assets/brand/deploy-button-split.svg)](https://levelrail.com/deploy?image=nginx)',
    )
  })
})
