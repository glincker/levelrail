// Tiny safe renderer model for announcement text. It returns tokens, never
// markup, so nothing in a feed can become HTML. Mirrors the backend subset.

export type InlineToken =
  | { kind: 'text'; text: string }
  | { kind: 'bold'; text: string }
  | { kind: 'code'; text: string }
  | { kind: 'link'; text: string; href: string }

const INLINE = /(\*\*[^*\n]+\*\*|`[^`\n]+`|\[[^\]\n]+\]\([^)\s]+\))/g
const LINK = /^\[([^\]\n]+)\]\(([^)\s]+)\)$/

/** Only https links to levelrail.com (and subdomains) or the glincker org on the code host. */
export function isAllowedLink(raw: string): boolean {
  let u: URL
  try {
    u = new URL(raw)
  } catch {
    return false
  }
  if (u.protocol !== 'https:' || u.username || u.password || u.port) {
    return false
  }
  const host = u.hostname.toLowerCase()
  if (host === 'levelrail.com' || host.endsWith('.levelrail.com')) return true
  return (
    host === 'github.com' && u.pathname.toLowerCase().startsWith('/glincker/')
  )
}

function inline(line: string): InlineToken[] {
  const out: InlineToken[] = []
  for (const part of line.split(INLINE)) {
    if (!part) continue
    if (part.startsWith('**') && part.endsWith('**') && part.length > 4) {
      out.push({ kind: 'bold', text: part.slice(2, -2) })
      continue
    }
    if (part.startsWith('`') && part.endsWith('`') && part.length > 2) {
      out.push({ kind: 'code', text: part.slice(1, -1) })
      continue
    }
    const m = LINK.exec(part)
    const label = m?.[1]
    const url = m?.[2]
    if (label && url && isAllowedLink(url)) {
      out.push({ kind: 'link', text: label, href: new URL(url).href })
      continue
    }
    out.push({ kind: 'text', text: part })
  }
  return out
}

/** Paragraphs split on blank lines; each is a list of inline tokens. */
export function parseAnnouncementText(src: string): InlineToken[][] {
  return src
    .replace(/\r\n?/g, '\n')
    .split(/\n{2,}/)
    .map((p) => p.trim())
    .filter(Boolean)
    .map(inline)
}

/** Plain text for places that cannot render tokens, such as the attention list. */
export function announcementPlainText(src: string): string {
  return parseAnnouncementText(src)
    .map((p) => p.map((t) => t.text).join(''))
    .join(' ')
}
