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

export interface DocPageHit {
  path: string
  title: string
  /** Up to a few headings of this page that match, in page order. */
  headings: DocHeading[]
  score: number
}

const TITLE_EXACT = 100
const TITLE_PART = 60
const HEADING = 40
const BODY = 10

// One row per page, best page first: title beats heading beats body, and a page
// is never listed twice however many of its headings match.
export function searchDocsGrouped(
  manifest: DocsManifest,
  query: string,
  limits: { pages: number; headings: number } = { pages: 8, headings: 2 },
): DocPageHit[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  const hits: DocPageHit[] = []
  for (const [path, page] of Object.entries(manifest.pages)) {
    if (path === '/') continue
    const title = page.title.toLowerCase()
    const headings = page.headings.filter((h) =>
      h.text.toLowerCase().includes(q),
    )
    let score = 0
    if (title === q) score = TITLE_EXACT
    else if (title.includes(q)) score = TITLE_PART
    else if (headings.length > 0) score = HEADING + Math.min(headings.length, 5)
    else if (page.body.toLowerCase().includes(q)) score = BODY
    if (score === 0) continue
    hits.push({
      path,
      title: page.title,
      headings: headings.slice(0, limits.headings),
      score,
    })
  }
  return hits
    .sort((a, b) => b.score - a.score || a.title.localeCompare(b.title))
    .slice(0, limits.pages)
}
