import { describe, expect, it } from 'vitest'
import { formatPublishedPort } from './publishedPort'

describe('formatPublishedPort', () => {
  it('shows host port with protocol', () => {
    expect(
      formatPublishedPort({
        container_port: 53,
        host_port: 5353,
        protocol: 'udp',
      }),
    ).toBe('5353/udp')
  })
})
