import { splitMatches } from '../../lib/runtimeSettings'

/** Wraps query matches in <mark> so a search result shows why it matched. */
export function HighlightedText({
  text,
  query,
}: {
  text: string
  query: string
}) {
  return (
    <>
      {splitMatches(text, query).map((part, i) =>
        part.match ? (
          <mark
            key={i}
            className="rounded-sm bg-tone-warning-soft px-0.5 text-foreground"
          >
            {part.text}
          </mark>
        ) : (
          <span key={i}>{part.text}</span>
        ),
      )}
    </>
  )
}
