import type { DocHeading, DocsManifest } from '../types/docs'

export interface DocSearchResult {
  path: string
  title: string
  /** Set when the match is a heading rather than the page title itself. */
  heading?: DocHeading
}

// Plain case-insensitive substring search over page titles and their
// headings, client-side, no index build: this docs set is ~30 pages, not
// large enough to justify a real search library.
export function searchDocs(
  manifest: DocsManifest,
  query: string,
): DocSearchResult[] {
  const q = query.trim().toLowerCase()
  if (!q) return []

  const results: DocSearchResult[] = []
  for (const [path, page] of Object.entries(manifest.pages)) {
    // The index page ("/") is docs/README.md, a table of contents that
    // duplicates the sidebar; searching it just surfaces category-name
    // matches, not real content.
    if (path === '/') continue
    if (page.title.toLowerCase().includes(q)) {
      results.push({ path, title: page.title })
    }
    for (const heading of page.headings) {
      if (heading.text.toLowerCase().includes(q)) {
        results.push({ path, title: page.title, heading })
      }
    }
  }
  return results
}
