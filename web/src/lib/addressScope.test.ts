import { describe, expect, it } from 'vitest'
import { classifyHost, isPrivateClass, type HostClass } from './addressScope'

const cases: Array<[string | undefined, HostClass]> = [
  ['72.61.8.70', 'public'],
  ['8.8.8.8', 'public'],
  ['172.15.0.1', 'public'],
  ['172.32.0.1', 'public'],
  ['10.181.0.1', 'private'],
  ['172.16.0.1', 'private'],
  ['172.31.255.1', 'private'],
  ['192.168.1.20', 'private'],
  ['100.64.0.1', 'cgnat'],
  ['100.127.255.1', 'cgnat'],
  ['100.128.0.1', 'public'],
  ['127.0.0.1', 'loopback'],
  ['localhost', 'loopback'],
  ['::1', 'loopback'],
  ['[::1]', 'loopback'],
  ['fd12:3456::1', 'private'],
  ['fc00::1', 'private'],
  ['fe80::1', 'linkLocal'],
  ['169.254.1.1', 'linkLocal'],
  ['2001:db8::1', 'public'],
  ['app.example.com', 'hostname'],
  ['999.1.1.1', 'hostname'],
  ['', 'none'],
  [undefined, 'none'],
]

describe('classifyHost', () => {
  it.each(cases)('%s is %s', (host, want) => {
    expect(classifyHost(host)).toBe(want)
  })
})

describe('isPrivateClass', () => {
  it('treats private, cgnat and link-local as same-network only', () => {
    expect(isPrivateClass('private')).toBe(true)
    expect(isPrivateClass('cgnat')).toBe(true)
    expect(isPrivateClass('linkLocal')).toBe(true)
    expect(isPrivateClass('public')).toBe(false)
    expect(isPrivateClass('loopback')).toBe(false)
  })
})
