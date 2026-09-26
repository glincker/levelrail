import { useEffect, useState } from 'react'
import { Suggestion, type SuggestionProps } from './Suggestion'
import { useReducedMotion } from './useReducedMotion'

export type SuggestionItem = SuggestionProps & { id: string }

export interface SuggestionListProps {
  items: SuggestionItem[]
  emptyLabel?: string
}

interface Entry {
  item: SuggestionItem
  exiting: boolean
}

const EXIT_MS = 200

function merge(
  prev: Entry[],
  items: SuggestionItem[],
  keepExiting: boolean,
): Entry[] {
  const byId = new Map(items.map((i) => [i.id, i]))
  const seen = new Set<string>()
  const next: Entry[] = []
  for (const e of prev) {
    const live = byId.get(e.item.id)
    if (live) {
      next.push({ item: live, exiting: false })
      seen.add(live.id)
    } else if (keepExiting) {
      next.push({ item: e.item, exiting: true })
    }
  }
  for (const i of items) {
    if (!seen.has(i.id)) next.push({ item: i, exiting: false })
  }
  return next
}

export function SuggestionList({ items, emptyLabel }: SuggestionListProps) {
  const reduced = useReducedMotion()
  const [state, setState] = useState<{
    src: SuggestionItem[]
    entries: Entry[]
  }>(() => ({ src: items, entries: merge([], items, false) }))

  if (state.src !== items) {
    setState({ src: items, entries: merge(state.entries, items, !reduced) })
  }

  const hasExiting = state.entries.some((e) => e.exiting)
  useEffect(() => {
    if (!hasExiting) return
    const t = window.setTimeout(
      () =>
        setState((s) => ({
          ...s,
          entries: s.entries.filter((e) => !e.exiting),
        })),
      EXIT_MS,
    )
    return () => window.clearTimeout(t)
  }, [hasExiting])

  const entries = state.entries
  if (entries.length === 0) {
    return emptyLabel ? (
      <p className="py-4 text-center text-sm text-muted-foreground">
        {emptyLabel}
      </p>
    ) : null
  }

  return (
    <ul className="flex flex-col gap-2" aria-label="Suggestions">
      {entries.map(({ item, exiting }) => (
        <li
          key={item.id}
          data-exiting={exiting || undefined}
          className={exiting ? 'kit-exit' : 'kit-enter'}
        >
          <Suggestion {...item} />
        </li>
      ))}
    </ul>
  )
}
