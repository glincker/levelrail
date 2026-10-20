import { describe, expect, it } from 'vitest'
import { isInsecureRemoteConnection } from './connection'

describe('isInsecureRemoteConnection', () => {
  it.each([
    ['http:', '203.0.113.7', true],
    ['http:', 'dash.example.com', true],
    ['https:', 'dash.example.com', false],
    ['http:', 'localhost', false],
    ['http:', '127.0.0.1', false],
    ['http:', '[::1]', false],
  ])('%s//%s -> %s', (protocol, hostname, want) => {
    expect(isInsecureRemoteConnection({ protocol, hostname })).toBe(want)
  })
})
