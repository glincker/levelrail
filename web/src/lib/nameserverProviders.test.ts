import { describe, expect, it } from 'vitest'
import {
  PROVIDER_DOCS,
  PROVIDER_LABELS,
  providerLabel,
  recordHost,
} from './nameserverProviders'

describe('providerLabel', () => {
  it('maps every detected id to its display name', () => {
    expect(providerLabel('digitalocean')).toBe('DigitalOcean')
    expect(providerLabel('google')).toBe('Google Cloud DNS')
    expect(providerLabel('desec')).toBe('deSEC')
    expect(providerLabel('ns1')).toBe('NS1')
  })

  it.each(['other', 'unknown', undefined, 'toString'])(
    'returns null for %s',
    (id) => {
      expect(providerLabel(id)).toBeNull()
    },
  )

  it('only links https docs', () => {
    for (const url of Object.values(PROVIDER_DOCS)) {
      expect(url.startsWith('https://')).toBe(true)
    }
    expect(Object.keys(PROVIDER_LABELS)).toHaveLength(11)
  })
})

describe('recordHost', () => {
  it('uses @ for the apex and the label for a subdomain', () => {
    expect(recordHost('example.com', 'example.com.')).toBe('@')
    expect(recordHost('app.example.com', 'example.com')).toBe('app')
    expect(recordHost('a.b.example.com', 'example.com')).toBe('a.b')
    expect(recordHost('*.example.com', 'example.com')).toBe('@')
  })
})
