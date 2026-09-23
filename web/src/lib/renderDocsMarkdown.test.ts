import { describe, expect, it } from 'vitest'
import { renderDocsMarkdown } from './renderDocsMarkdown'
import type { DocsManifest } from '../types/docs'

const manifest: DocsManifest = {
  categories: [],
  pages: {
    '/getting-started': {
      file: 'getting-started.md',
      title: 'Getting started',
      headings: [],
    },
  },
}

describe('renderDocsMarkdown', () => {
  it('strips VitePress frontmatter before rendering', () => {
    const html = renderDocsMarkdown(
      '---\ndescription: hello\n---\n\n# Title\n',
      'x.md',
      manifest,
      '',
    )
    expect(html).not.toContain('description: hello')
    expect(html).toContain('Title')
  })

  it('assigns a slug id to headings', () => {
    const html = renderDocsMarkdown(
      '## Cordon, drain, uncordon\n',
      'x.md',
      manifest,
      '',
    )
    expect(html).toContain('id="cordon-drain-uncordon"')
  })

  it('renders a ::: tip container as a callout, not raw fence syntax', () => {
    const html = renderDocsMarkdown(
      '::: tip\nDo the thing.\n:::\n',
      'x.md',
      manifest,
      '',
    )
    expect(html).not.toContain(':::')
    expect(html).toContain('Tip')
    expect(html).toContain('Do the thing.')
  })

  it('renders a ::: details container as a native <details> with its title as the summary', () => {
    const html = renderDocsMarkdown(
      '::: details My summary\nHidden body.\n:::\n',
      'x.md',
      manifest,
      '',
    )
    expect(html).toContain('<details')
    expect(html).toContain('<summary')
    expect(html).toContain('My summary')
    expect(html).toContain('Hidden body.')
  })

  it('rewrites an internal .md link to the in-app /help route', () => {
    const html = renderDocsMarkdown(
      '[Getting started](getting-started.md)',
      'installing.md',
      manifest,
      '',
    )
    expect(html).toContain('href="/help/getting-started"')
    expect(html).not.toContain('target="_blank"')
  })

  it('opens an external link in a new tab', () => {
    const html = renderDocsMarkdown(
      '[GitHub](https://github.com/glincker/levelrail)',
      'installing.md',
      manifest,
      '',
    )
    expect(html).toContain('target="_blank"')
    expect(html).toContain('rel="noreferrer"')
  })

  it('renders an unresolved internal link as plain text, not a dead link', () => {
    const html = renderDocsMarkdown(
      '[ADR 5](../adr/005-caddy-embedded-ingress.md)',
      'installing.md',
      manifest,
      '',
    )
    expect(html).not.toContain('<a ')
    expect(html).toContain('ADR 5')
  })
})
