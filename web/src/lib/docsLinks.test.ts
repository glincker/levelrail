import { describe, expect, it } from 'vitest'
import { resolveDocLink } from './docsLinks'
import type { DocsManifest } from '../types/docs'

const manifest: DocsManifest = {
  categories: [],
  pages: {
    '/getting-started': {
      file: 'getting-started.md',
      title: 'Getting started',
      headings: [],
    },
    '/troubleshooting': {
      file: 'troubleshooting.md',
      title: 'Troubleshooting',
      headings: [],
    },
    '/design/git-provider-integrations': {
      file: 'design/git-provider-integrations.md',
      title: 'Git provider integrations',
      headings: [],
    },
  },
}

describe('resolveDocLink', () => {
  it('rewrites a same-directory relative .md link to an in-app route', () => {
    const result = resolveDocLink(
      'getting-started.md',
      'installing.md',
      manifest,
      '',
    )
    expect(result).toEqual({
      href: '/help/getting-started',
      external: false,
      linkable: true,
    })
  })

  it('rewrites a ./ prefixed link the same way', () => {
    const result = resolveDocLink(
      './troubleshooting.md',
      'installing.md',
      manifest,
      '',
    )
    expect(result.href).toBe('/help/troubleshooting')
    expect(result.external).toBe(false)
  })

  it('resolves a ../ link relative to a nested doc', () => {
    const result = resolveDocLink(
      '../troubleshooting.md',
      'design/git-provider-integrations.md',
      manifest,
      '',
    )
    expect(result.href).toBe('/help/troubleshooting')
  })

  it('preserves an anchor on an internal link', () => {
    const result = resolveDocLink(
      'troubleshooting.md#fix-it',
      'getting-started.md',
      manifest,
      '',
    )
    expect(result.href).toBe('/help/troubleshooting#fix-it')
  })

  it('leaves a bare in-page anchor untouched', () => {
    const result = resolveDocLink(
      '#some-heading',
      'getting-started.md',
      manifest,
      '',
    )
    expect(result).toEqual({
      href: '#some-heading',
      external: false,
      linkable: true,
    })
  })

  it('leaves an absolute http(s) URL untouched and marks it external', () => {
    const result = resolveDocLink(
      'https://example.com/x',
      'getting-started.md',
      manifest,
      '',
    )
    expect(result).toEqual({
      href: 'https://example.com/x',
      external: true,
      linkable: true,
    })
  })

  it('falls back to the hosted docs URL for an unbundled internal link', () => {
    const result = resolveDocLink(
      '../adr/005-caddy-embedded-ingress.md',
      'installing.md',
      manifest,
      'https://glinr.com/levelrail',
    )
    expect(result).toEqual({
      href: 'https://glinr.com/levelrail/adr/005-caddy-embedded-ingress',
      external: true,
      linkable: true,
    })
  })

  it('is not linkable for an unbundled internal link with no docs URL configured', () => {
    const result = resolveDocLink(
      '../adr/005-caddy-embedded-ingress.md',
      'installing.md',
      manifest,
      '',
    )
    expect(result.linkable).toBe(false)
  })

  it('leaves a non-markdown relative link alone', () => {
    const result = resolveDocLink(
      './diagram.png',
      'getting-started.md',
      manifest,
      '',
    )
    expect(result).toEqual({
      href: './diagram.png',
      external: false,
      linkable: true,
    })
  })
})
