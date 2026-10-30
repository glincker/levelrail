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
  /** Where the hosted docs site files this page. */
  section?: string
}

const WORD_TITLE_EXACT = 60
const WORD_TITLE = 30
const WORD_HEADING = 15
const WORD_BODY = 3
const PHRASE_IN_TITLE = 40
const PHRASE_IS_TITLE = 100

// Word-level search: every word must appear in the title, a heading or the body excerpt,
// in any order, and a page is listed once however many of its headings match.
export function searchDocsGrouped(
  manifest: DocsManifest,
  query: string,
  limits: { pages: number; headings: number } = { pages: 8, headings: 2 },
): DocPageHit[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  const words = q.split(/\s+/).filter(Boolean)
  const hits: DocPageHit[] = []
  // When the manifest knows what the hosted site publishes, internal pages stay out of results.
  const filterPublished = Object.values(manifest.pages).some((p) => p.section)
  for (const [path, page] of Object.entries(manifest.pages)) {
    if (path === '/') continue
    if (filterPublished && !page.section) continue
    const title = page.title.toLowerCase()
    const body = page.body.toLowerCase()
    const headings = page.headings.map((heading, order) => ({
      heading,
      order,
      text: heading.text.toLowerCase(),
    }))
    let score = 0
    let matchesAll = true
    for (const word of words) {
      let w = 0
      if (title === word) w = WORD_TITLE_EXACT
      else if (title.includes(word)) w = WORD_TITLE
      else if (headings.some((h) => h.text.includes(word))) w = WORD_HEADING
      else if (body.includes(word)) w = WORD_BODY
      if (w === 0) {
        matchesAll = false
        break
      }
      score += w
    }
    if (!matchesAll) continue
    if (title === q) score += PHRASE_IS_TITLE
    else if (title.includes(q)) score += PHRASE_IN_TITLE
    const shown = headings
      .map((h) => ({
        ...h,
        hit: words.filter((w) => h.text.includes(w)).length,
      }))
      .filter((h) => h.hit > 0)
      .sort((a, b) => b.hit - a.hit || a.order - b.order)
      .slice(0, limits.headings)
      .sort((a, b) => a.order - b.order)
      .map((h) => h.heading)
    hits.push({
      path,
      title: page.title,
      headings: shown,
      score,
      section: page.section,
    })
  }
  return hits
    .sort((a, b) => b.score - a.score || a.title.localeCompare(b.title))
    .slice(0, limits.pages)
}
