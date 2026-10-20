import type { DocHeading, DocsManifest } from '../types/docs'

export interface DocSearchResult {
  path: string
  title: string
  /** Set when the match is a heading rather than the page title itself. */
  heading?: DocHeading
}

// Plain case-insensitive substring search over page titles, headings, and
// a body excerpt (manifest's `body` field, see vite-plugins/docsManifest.ts),
// client-side, no index build: this docs set is ~30 pages, not large enough
// to justify a real search library.
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
    let matched = false
    if (page.title.toLowerCase().includes(q)) {
      results.push({ path, title: page.title })
      matched = true
    }
    for (const heading of page.headings) {
      if (heading.text.toLowerCase().includes(q)) {
        results.push({ path, title: page.title, heading })
        matched = true
      }
    }
    // Falls back to the body excerpt only when title/headings found
    // nothing, so a page already surfaced by a heading match doesn't also
    // show a redundant page-level entry.
    if (!matched && page.body.toLowerCase().includes(q)) {
      results.push({ path, title: page.title })
    }
  }
  return results
}
