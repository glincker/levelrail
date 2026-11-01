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
  /** Only set by full fixtures; the shipped manifest omits it and search text lives in DocsSearchBodies. */
  body?: string
  /** "Tab / Group" on the hosted docs site; absent for pages the site does not publish. */
  section?: string
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

/** Plain-text search excerpt per in-app path, loaded lazily (virtual:docs-search-bodies). */
export type DocsSearchBodies = Readonly<Record<string, string>>
