import { describe, expect, it } from 'vitest'
import { latestNewerTag, parseDockerHubImage } from './imageUpdate'

describe('parseDockerHubImage', () => {
  it.each([
    [
      'axllent/mailpit:v1.21.8',
      { namespace: 'axllent', repo: 'mailpit', tag: 'v1.21.8' },
    ],
    [
      'postgres:16-alpine',
      { namespace: 'library', repo: 'postgres', tag: '16-alpine' },
    ],
    ['nginx', { namespace: 'library', repo: 'nginx', tag: 'latest' }],
    [
      'grafana/grafana:11.0.0@sha256:abc',
      { namespace: 'grafana', repo: 'grafana', tag: '11.0.0' },
    ],
  ])('parses %s', (image, want) => {
    expect(parseDockerHubImage(image)).toEqual(want)
  })

  it.each([
    'ghcr.io/plausible/community-edition:v3.0.1',
    'registry.example.com:5000/app:1.0',
    'localhost/app:1',
    'quay.io/org/app:1',
  ])('returns null for another registry: %s', (image) => {
    expect(parseDockerHubImage(image)).toBeNull()
  })
})

describe('latestNewerTag', () => {
  const tags = [
    'latest',
    'v1.21.8',
    'v1.22.0',
    'v1.22.1',
    'v1.22.2-rc1',
    'v2.0.0',
    'main',
    'v1.21.9',
  ]

  it('picks the highest newer tag in the same family', () => {
    expect(latestNewerTag('v1.21.8', tags)).toBe('v2.0.0')
  })

  it('ignores pre-releases unless already on one', () => {
    expect(latestNewerTag('v1.22.1', ['v1.22.2-rc1'])).toBeNull()
    expect(latestNewerTag('v1.22.1-rc1', ['v1.22.2-rc1'])).toBe('v1.22.2-rc1')
    expect(latestNewerTag('v1.22.1-rc1', ['v1.22.1-rc2'])).toBeNull()
  })

  it('keeps the suffix family, so alpine never jumps to a plain tag', () => {
    expect(latestNewerTag('16-alpine', ['16', '17', '17-alpine', '18'])).toBe(
      '17-alpine',
    )
  })

  it('does not suggest an older or equal version, or a different v prefix', () => {
    expect(latestNewerTag('v2.0.0', ['v1.9.0', 'v2.0.0'])).toBeNull()
    expect(latestNewerTag('1.2.3', ['v1.2.4'])).toBeNull()
  })

  it('compares numerically, not as strings', () => {
    expect(latestNewerTag('1.9.0', ['1.10.0', '1.9.9'])).toBe('1.10.0')
  })

  it('has nothing to say about latest or non-version tags', () => {
    expect(latestNewerTag('latest', tags)).toBeNull()
    expect(latestNewerTag('main', tags)).toBeNull()
  })
})
