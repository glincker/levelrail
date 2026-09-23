import { describe, expect, it } from 'vitest'
import { searchDocs } from './docsSearch'
import type { DocsManifest } from '../types/docs'

const manifest: DocsManifest = {
  categories: [],
  pages: {
    '/': {
      file: 'README.md',
      title: 'Levelrail docs',
      headings: [{ id: 'index', text: 'Index', level: 2 }],
    },
    '/multi-node': {
      file: 'multi-node.md',
      title: 'Multi-node',
      headings: [
        {
          id: 'cordon-drain-uncordon',
          text: 'Cordon, drain, uncordon',
          level: 2,
        },
        {
          id: 'rotating-the-control-planes-mesh-key',
          text: "Rotating the control plane's mesh key",
          level: 3,
        },
      ],
    },
    '/troubleshooting': {
      file: 'troubleshooting.md',
      title: 'Troubleshooting',
      headings: [
        {
          id: 'tls-certificate-wont-issue',
          text: "TLS certificate won't issue",
          level: 3,
        },
      ],
    },
  },
}

describe('searchDocs', () => {
  it('returns nothing for an empty query', () => {
    expect(searchDocs(manifest, '')).toEqual([])
    expect(searchDocs(manifest, '   ')).toEqual([])
  })

  it('matches a page title case-insensitively', () => {
    const results = searchDocs(manifest, 'troubleshoot')
    expect(results).toContainEqual({
      path: '/troubleshooting',
      title: 'Troubleshooting',
    })
  })

  it('matches a heading and includes it in the result', () => {
    const results = searchDocs(manifest, 'mesh key')
    expect(results).toContainEqual({
      path: '/multi-node',
      title: 'Multi-node',
      heading: {
        id: 'rotating-the-control-planes-mesh-key',
        text: "Rotating the control plane's mesh key",
        level: 3,
      },
    })
  })

  it('never returns a result for the index page ("/")', () => {
    const results = searchDocs(manifest, 'index')
    expect(results.some((r) => r.path === '/')).toBe(false)
  })

  it('returns no results for a query that matches nothing', () => {
    expect(searchDocs(manifest, 'xyznotarealterm')).toEqual([])
  })
})
