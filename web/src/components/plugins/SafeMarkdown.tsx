import type { ReactNode } from 'react'
import { safeHref } from '../../lib/safeHref'

const INLINE = /\*\*([^*]+)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)/g

function inline(text: string, keyPrefix: string): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let n = 0
  for (const m of text.matchAll(INLINE)) {
    const at = m.index ?? 0
    if (at > last) out.push(text.slice(last, at))
    const key = `${keyPrefix}-${n++}`
    if (m[1] !== undefined) {
      out.push(<strong key={key}>{m[1]}</strong>)
    } else if (m[2] !== undefined) {
      out.push(
        <code key={key} className="rounded bg-muted px-1 font-mono text-xs">
          {m[2]}
        </code>,
      )
    } else {
      const href = safeHref(m[4] ?? '')
      out.push(
        href ? (
          <a
            key={key}
            href={href}
            target="_blank"
            rel="noopener noreferrer nofollow"
            className="underline underline-offset-2"
          >
            {m[3]}
          </a>
        ) : (
          (m[3] ?? '')
        ),
      )
    }
    last = at + m[0].length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

/**
 * Renders plugin text as a small markdown subset (paragraphs, dash lists, bold,
 * code, http(s) links) into React nodes. It never produces raw HTML.
 */
export function SafeMarkdown({ text }: Readonly<{ text: string }>) {
  const blocks: ReactNode[] = []
  let list: string[] = []
  const flush = () => {
    if (list.length === 0) return
    const items = list
    list = []
    blocks.push(
      <ul key={`ul-${blocks.length}`} className="list-disc space-y-1 pl-5">
        {items.map((item, i) => (
          <li key={i}>{inline(item, `li-${blocks.length}-${i}`)}</li>
        ))}
      </ul>,
    )
  }
  for (const line of text.split('\n')) {
    const bullet = /^\s*[-*]\s+(.*)$/.exec(line)
    if (bullet) {
      list.push(bullet[1] ?? '')
      continue
    }
    flush()
    if (line.trim() !== '') {
      blocks.push(
        <p key={`p-${blocks.length}`}>{inline(line, `p-${blocks.length}`)}</p>,
      )
    }
  }
  flush()
  return <div className="space-y-2 text-sm break-words">{blocks}</div>
}
