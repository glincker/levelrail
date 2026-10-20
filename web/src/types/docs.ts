// Shape of the 'virtual:docs-manifest' module (vite-plugins/docsManifest.
// mjs builds it from /docs at build/dev/test time). Only the /help route's
// own lazy chunk loads this (see docsManifestLoader.ts), never the main
// bundle, so `body` can hold a search-sized excerpt without it costing
// anything on first load.
export interface DocHeading {
  id: string
  text: string
  level: 2 | 3
}

export interface DocPageMeta {
  /** Doc-relative source path, e.g. "getting-started.md" or "design/git-provider-integrations.md". */
  file: string
  title: string
  headings: DocHeading[]
  /** Plain-text excerpt of the page's paragraph content, for full-text search (see docsSearch.ts). */
  body: string
}

export interface DocCategory {
  name: string
  /** In README.md's own index order. */
  docs: { path: string; title: string }[]
}

export interface DocsManifest {
  categories: DocCategory[]
  /** Keyed by in-app path, e.g. "/getting-started", "/" for the index. */
  pages: Record<string, DocPageMeta>
}
