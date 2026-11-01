// Ambient declarations for the two virtual modules vite-plugins/docsManifest.mjs
// registers (see that file for why they're virtual rather than a committed
// generated file).
declare module 'virtual:docs-manifest' {
  import type { DocsManifest } from './docs'

  const docsManifest: DocsManifest
  export default docsManifest
}

declare module 'virtual:docs-search-bodies' {
  import type { DocsSearchBodies } from './docs'

  const bodies: DocsSearchBodies
  export default bodies
}

declare module 'virtual:docs-path-index' {
  const docsPathIndex: ReadonlySet<string>
  export default docsPathIndex
}
