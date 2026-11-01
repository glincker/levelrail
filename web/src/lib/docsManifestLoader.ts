import type { DocsManifest, DocsSearchBodies } from '../types/docs'

// One typed wrapper around the dynamic import of the manifest virtual
// module, used by every /help route's loader instead of importing it
// statically (see routes/help.tsx's own comment for why: this route
// shell is reachable from the always-loaded app chrome, and a static
// import would pull all 32 pages' headings into the main bundle).
export function loadDocsManifest(): Promise<DocsManifest> {
  return (
    import('virtual:docs-manifest') as Promise<{ default: DocsManifest }>
  ).then((mod) => mod.default)
}

// Search text is split out so the eager manifest chunk stays small; fetched
// on the first search from the bundled asset, no network beyond our own origin.
export function loadDocsSearchBodies(): Promise<DocsSearchBodies> {
  return (
    import('virtual:docs-search-bodies') as Promise<{
      default: DocsSearchBodies
    }>
  ).then((mod) => mod.default)
}
