import { describe, expect, it } from 'vitest'
import {
  certificatePath,
  isValidDomain,
  redirectPair,
  suggestPort,
  wwwPair,
} from './domainWizard'

describe('wwwPair', () => {
  it.each([
    [
      'example.com',
      { apex: 'example.com', www: 'www.example.com', typed: 'apex' },
    ],
    [
      'www.example.com',
      { apex: 'example.com', www: 'www.example.com', typed: 'www' },
    ],
    [
      'WWW.Example.COM',
      { apex: 'example.com', www: 'www.example.com', typed: 'www' },
    ],
    ['app.example.com', null],
    ['localhost', null],
    ['', null],
  ])('%s', (input, want) => {
    expect(wwwPair(input)).toEqual(want)
  })
})

describe('redirectPair', () => {
  it('redirects www to a typed apex', () => {
    const pair = wwwPair('example.com')!
    expect(redirectPair(pair)).toEqual({
      from: 'www.example.com',
      to: 'example.com',
    })
  })
  it('redirects apex to a typed www', () => {
    const pair = wwwPair('www.example.com')!
    expect(redirectPair(pair)).toEqual({
      from: 'example.com',
      to: 'www.example.com',
    })
  })
})

describe('suggestPort', () => {
  it.each([
    [3000, [], { port: 3000 }],
    [3000, [22, 3000], { port: 3000 }],
    [3000, [8080], { port: 3000, switchTo: 8080 }],
  ])('configured %i listening %j', (configured, listening, want) => {
    expect(suggestPort(configured, listening)).toEqual(want)
  })
})

describe('certificatePath', () => {
  it('wildcards always need dns-01', () => {
    expect(certificatePath('*.example.com', 'http-01', false)).toBe(
      'dns-01-wildcard',
    )
  })
  it('private address needs dns-01', () => {
    expect(certificatePath('a.example.com', 'dns-01-required', true)).toBe(
      'dns-01-private',
    )
  })
  it('public host uses http-01', () => {
    expect(certificatePath('a.example.com', 'http-01', false)).toBe('http-01')
  })
})

describe('isValidDomain', () => {
  it('rejects a bare label', () => {
    expect(isValidDomain('localhost')).toBe(false)
    expect(isValidDomain('a.example.com')).toBe(true)
  })
})
