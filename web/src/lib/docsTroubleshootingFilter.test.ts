import { describe, it, expect } from 'vitest'
import { getTroubleshootingPages } from './docsTroubleshootingFilter'
import type { DocsManifest } from '@/types/docs'

describe('docsTroubleshootingFilter', () => {
  it('finds pages with Troubleshooting section', () => {
    const manifest: DocsManifest = {
      categories: [],
      pages: {
        '/getting-started': {
          file: 'getting-started.md',
          title: 'Getting Started',
          headings: [
            { id: 'intro', text: 'Introduction', level: 2 },
            { id: 'troubleshooting', text: 'Troubleshooting', level: 2 },
          ],
          body: 'content',
        },
        '/deployment': {
          file: 'deployment.md',
          title: 'Deployment',
          headings: [{ id: 'intro', text: 'Introduction', level: 2 }],
          body: 'content',
        },
      },
    }

    const result = getTroubleshootingPages(manifest)
    expect(result.has('/getting-started')).toBe(true)
    expect(result.has('/deployment')).toBe(false)
  })

  it('is case-insensitive', () => {
    const manifest: DocsManifest = {
      categories: [],
      pages: {
        '/page1': {
          file: 'page1.md',
          title: 'Page 1',
          headings: [{ id: 'ts', text: 'TROUBLESHOOTING', level: 2 }],
          body: 'content',
        },
      },
    }

    const result = getTroubleshootingPages(manifest)
    expect(result.has('/page1')).toBe(true)
  })

  it('requires level 2 heading', () => {
    const manifest: DocsManifest = {
      categories: [],
      pages: {
        '/page1': {
          file: 'page1.md',
          title: 'Page 1',
          headings: [{ id: 'ts', text: 'Troubleshooting', level: 3 }],
          body: 'content',
        },
      },
    }

    const result = getTroubleshootingPages(manifest)
    expect(result.has('/page1')).toBe(false)
  })

  it('handles empty pages', () => {
    const manifest: DocsManifest = {
      categories: [],
      pages: {},
    }

    const result = getTroubleshootingPages(manifest)
    expect(result.size).toBe(0)
  })

  it('returns a Set', () => {
    const manifest: DocsManifest = {
      categories: [],
      pages: {
        '/page1': {
          file: 'page1.md',
          title: 'Page 1',
          headings: [{ id: 'ts', text: 'Troubleshooting', level: 2 }],
          body: 'content',
        },
      },
    }

    const result = getTroubleshootingPages(manifest)
    expect(result instanceof Set).toBe(true)
  })
})
