// Shape of the 'virtual:docs-manifest' module (vite-plugins/docsManifest.
// mjs builds it from /docs at build/dev/test time). Titles and headings
// only, never full page bodies, so this stays small enough to import
// eagerly from HelpLink and the /help layout alike.
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
