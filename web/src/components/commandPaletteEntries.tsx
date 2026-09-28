import * as React from 'react'
import { cn } from '@/lib/utils'
import { fuzzyMatchIndices } from '@/lib/fuzzy'
import type { PaletteItem } from './commandPaletteData'

function Highlighted({ text, query }: { text: string; query: string }) {
  const hit = new Set(fuzzyMatchIndices(query, text))
  if (hit.size === 0) return <>{text}</>
  const runs: { text: string; hit: boolean }[] = []
  Array.from(text).forEach((ch, i) => {
    const h = hit.has(i)
    const last = runs[runs.length - 1]
    if (last && last.hit === h) last.text += ch
    else runs.push({ text: ch, hit: h })
  })
  return (
    <>
      {runs.map((r, i) =>
        r.hit ? (
          <mark key={i} className="bg-transparent font-semibold text-primary">
            {r.text}
          </mark>
        ) : (
          r.text
        ),
      )}
    </>
  )
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded border border-border bg-background px-1 font-mono text-[10px] leading-4 text-muted-foreground">
      {children}
    </kbd>
  )
}

export function ResultRow({
  item,
  optionId,
  active,
  query = '',
  onSelect,
}: {
  item: PaletteItem
  query?: string
  optionId: string
  active: boolean
  onSelect: () => void
}) {
  const ref = React.useRef<HTMLButtonElement>(null)
  React.useEffect(() => {
    if (active) ref.current?.scrollIntoView?.({ block: 'nearest' })
  }, [active])

  return (
    <button
      ref={ref}
      type="button"
      id={optionId}
      role="option"
      aria-selected={active}
      data-active={active}
      tabIndex={-1}
      onClick={onSelect}
      className={cn(
        'flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-sm outline-none',
        active
          ? 'bg-muted text-foreground'
          : 'text-foreground/90 hover:bg-muted/60',
      )}
    >
      <span className="flex size-4 shrink-0 items-center justify-center text-muted-foreground [&_svg]:size-4">
        {item.icon}
      </span>
      <span className="truncate">
        <Highlighted text={item.label} query={query} />
      </span>
      {active ? (
        <span className="ml-auto" aria-hidden="true">
          <Kbd>Enter</Kbd>
        </span>
      ) : item.hint ? (
        <span className="ml-auto flex gap-0.5" aria-hidden="true">
          {item.hint.map((k) => (
            <Kbd key={k}>{k}</Kbd>
          ))}
        </span>
      ) : null}
    </button>
  )
}

export function PaletteFooter() {
  return (
    <div
      aria-hidden="true"
      className="flex items-center gap-3 border-t border-border px-3 py-2 text-xs text-muted-foreground"
    >
      <span className="flex items-center gap-1">
        <Kbd>Up</Kbd>
        <Kbd>Down</Kbd>
        navigate
      </span>
      <span className="flex items-center gap-1">
        <Kbd>Enter</Kbd>
        select
      </span>
      <span className="flex items-center gap-1">
        <Kbd>Esc</Kbd>
        close
      </span>
    </div>
  )
}
