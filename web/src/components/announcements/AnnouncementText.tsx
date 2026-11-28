import { useMemo } from 'react'
import { parseAnnouncementText } from '@/lib/announcementMarkdown'
import { cn } from '@/lib/utils'

// Renders the safe token model as React nodes, so no feed text is ever HTML.
export function AnnouncementText({
  text,
  firstOnly = false,
  className,
}: {
  text: string
  /** Show only the first paragraph, for the one-line banner. */
  firstOnly?: boolean
  className?: string
}) {
  const paragraphs = useMemo(() => {
    const all = parseAnnouncementText(text)
    return firstOnly ? all.slice(0, 1) : all
  }, [text, firstOnly])
  return (
    <div className={cn('space-y-1.5', className)}>
      {paragraphs.map((tokens, i) => (
        <p key={i}>
          {tokens.map((tk, j) => {
            if (tk.kind === 'bold') return <strong key={j}>{tk.text}</strong>
            if (tk.kind === 'code') {
              return (
                <code
                  key={j}
                  className="rounded bg-muted px-1 font-mono text-[0.9em]"
                >
                  {tk.text}
                </code>
              )
            }
            if (tk.kind === 'link') {
              return (
                <a
                  key={j}
                  href={tk.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="underline underline-offset-2"
                >
                  {tk.text}
                </a>
              )
            }
            return <span key={j}>{tk.text}</span>
          })}
        </p>
      ))}
    </div>
  )
}
