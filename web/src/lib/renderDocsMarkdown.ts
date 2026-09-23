import { Marked } from 'marked'
import type { Tokens } from 'marked'
import { stripFrontmatter } from './frontmatter'
import { resolveDocLink } from './docsLinks'
import { uniqueSlug } from './slugify'
import { vitepressContainerExtension } from './vitepressContainers'
import type { DocsManifest } from '../types/docs'

interface RenderState {
  currentFile: string
  manifest: DocsManifest
  docsBaseUrl: string
  headingIds: Map<string, number>
}

const emptyManifest: DocsManifest = { categories: [], pages: {} }
let state: RenderState = {
  currentFile: '',
  manifest: emptyManifest,
  docsBaseUrl: '',
  headingIds: new Map(),
}

// One Marked instance for the whole app, with custom renderers reading
// mutable `state` set fresh at the top of each renderDocsMarkdown call.
// Safe because rendering is synchronous and single-threaded; not safe to
// call renderDocsMarkdown concurrently from a worker, which this app
// never does.
const md = new Marked({ gfm: true })
md.use({
  extensions: [vitepressContainerExtension],
  renderer: {
    heading(token: Tokens.Heading) {
      const id = uniqueSlug(token.text, state.headingIds)
      const inner = this.parser.parseInline(token.tokens)
      return `<h${token.depth} id="${id}" class="group scroll-mt-20 font-semibold text-foreground">${inner}</h${token.depth}>`
    },
    link(token: Tokens.Link) {
      const resolved = resolveDocLink(
        token.href,
        state.currentFile,
        state.manifest,
        state.docsBaseUrl,
      )
      const inner = this.parser.parseInline(token.tokens)
      if (!resolved.linkable) {
        return `<span class="text-muted-foreground">${inner}</span>`
      }
      const attrs = resolved.external
        ? ' target="_blank" rel="noreferrer"'
        : resolved.href.startsWith('#')
          ? ''
          : ' data-internal-doc="true"'
      return `<a href="${resolved.href}"${attrs} class="text-primary underline underline-offset-2 hover:no-underline">${inner}</a>`
    },
  },
})

export function renderDocsMarkdown(
  markdown: string,
  currentFile: string,
  manifest: DocsManifest,
  docsBaseUrl: string,
): string {
  state = { currentFile, manifest, docsBaseUrl, headingIds: new Map() }
  return md.parse(stripFrontmatter(markdown), { async: false })
}
