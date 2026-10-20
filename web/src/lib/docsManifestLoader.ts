import type { DocsManifest } from '../types/docs'

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
