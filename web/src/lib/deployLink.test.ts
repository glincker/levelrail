import { describe, expect, it } from 'vitest'
import {
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
