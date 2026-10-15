import { describe, it, expect } from 'vitest'
import {
  filePathToRoutePath,
  routePathToDocsSiteUrl,
  routePathWithHash,
} from './docsPathMapper'

describe('docsPathMapper', () => {
  describe('filePathToRoutePath', () => {
    it('converts index.md to /', () => {
      expect(filePathToRoutePath('index.md')).toBe('/')
    })

    it('removes .md extension', () => {
      expect(filePathToRoutePath('getting-started.md')).toBe('/getting-started')
    })

    it('handles nested paths', () => {
      expect(filePathToRoutePath('guides/deployment.md')).toBe(
        '/guides/deployment',
      )
    })

    it('handles path already with leading slash', () => {
      expect(filePathToRoutePath('/getting-started.md')).toBe(
        '/getting-started',
      )
    })

    it('handles path without .md extension', () => {
      expect(filePathToRoutePath('about')).toBe('/about')
    })
  })

  describe('routePathToDocsSiteUrl', () => {
    const docsBaseUrl = 'https://docs.example.com'

    it('returns empty string when docsBaseUrl is empty', () => {
      expect(routePathToDocsSiteUrl('/', '')).toBe('')
    })

    it('maps root path to base URL', () => {
      expect(routePathToDocsSiteUrl('/', docsBaseUrl)).toBe(
        'https://docs.example.com',
      )
    })

    it('appends route path to base URL', () => {
      expect(routePathToDocsSiteUrl('/getting-started', docsBaseUrl)).toBe(
        'https://docs.example.com/getting-started',
      )
    })

    it('handles base URL with trailing slash', () => {
      expect(
        routePathToDocsSiteUrl(
          '/guides/deployment',
          'https://docs.example.com/',
        ),
      ).toBe('https://docs.example.com/guides/deployment')
    })
  })

  describe('routePathWithHash', () => {
    it('returns path only when hash is not provided', () => {
      expect(routePathWithHash('/getting-started')).toBe('/getting-started')
    })

    it('appends hash to path', () => {
      expect(routePathWithHash('/getting-started', 'troubleshooting')).toBe(
        '/getting-started#troubleshooting',
      )
    })

    it('handles root path with hash', () => {
      expect(routePathWithHash('/', 'section')).toBe('/#section')
    })

    it('handles empty hash', () => {
      expect(routePathWithHash('/page', '')).toBe('/page')
    })
  })
})
