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
      body: 'This index also happens to mention rollback strategy in passing.',
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
      body: 'Nodes join the WireGuard mesh automatically once enrolled.',
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
      body: 'If a deploy fails, check the rollback target is still pinned before retrying.',
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

  it('matches body text that appears in neither the title nor a heading', () => {
    const results = searchDocs(manifest, 'wireguard mesh')
    expect(results).toContainEqual({ path: '/multi-node', title: 'Multi-node' })
  })

  it('matches body text on a page whose title/headings do not otherwise match', () => {
    const results = searchDocs(manifest, 'pinned before retrying')
    expect(results).toEqual([
      { path: '/troubleshooting', title: 'Troubleshooting' },
    ])
  })

  it('does not add a redundant body-match entry when a heading already matched', () => {
    // "mesh" matches both the "...mesh key" heading and the page's body
    // excerpt; only the heading-level entry should show up.
    const results = searchDocs(manifest, 'mesh')
    expect(results).toEqual([
      {
        path: '/multi-node',
        title: 'Multi-node',
        heading: {
          id: 'rotating-the-control-planes-mesh-key',
          text: "Rotating the control plane's mesh key",
          level: 3,
        },
      },
    ])
  })

  it('never returns a result for the index page ("/")', () => {
    const results = searchDocs(manifest, 'index')
    expect(results.some((r) => r.path === '/')).toBe(false)
  })

  it('returns no results for a query that matches nothing', () => {
    expect(searchDocs(manifest, 'xyznotarealterm')).toEqual([])
  })
})
