import * as React from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { PaletteItem } from './commandPaletteData'
import { ResultRow } from './commandPaletteEntries'

export type PaletteEntry =
  | { type: 'header'; key: string; label: string }
  | { type: 'item'; key: string; item: PaletteItem; index: number }

export const VIRTUALIZE_ABOVE = 50
const ROW_HEIGHT = 32
const LIST_HEIGHT = 320

function Header({ label }: { label: string }) {
  return (
    <p
      aria-hidden="true"
      className="px-2.5 py-1 text-xs font-medium text-muted-foreground"
    >
      {label}
    </p>
  )
}

function VirtualEntries({
  entries,
  activeIndex,
  query,
  optionId,
  onSelect,
  scrollRef,
}: ListProps & { scrollRef: React.RefObject<HTMLDivElement | null> }) {
  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
    initialRect: { width: 480, height: LIST_HEIGHT },
  })
  const activeEntry = entries.findIndex(
    (e) => e.type === 'item' && e.index === activeIndex,
  )
  React.useEffect(() => {
    if (activeEntry >= 0) virtualizer.scrollToIndex(activeEntry)
  }, [activeEntry, virtualizer])

  return (
    <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
      {virtualizer.getVirtualItems().map((row) => {
        const entry = entries[row.index]
        if (!entry) return null
        return (
          <div
            key={entry.key}
            className="absolute top-0 left-0 w-full"
            style={{ transform: `translateY(${row.start}px)` }}
          >
            {entry.type === 'header' ? (
              <Header label={entry.label} />
            ) : (
              <ResultRow
                item={entry.item}
                optionId={optionId(entry.item)}
                active={entry.index === activeIndex}
                query={query}
                onSelect={() => onSelect(entry.item)}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

interface ListProps {
  entries: readonly PaletteEntry[]
  activeIndex: number
  query: string
  optionId: (item: PaletteItem) => string
  onSelect: (item: PaletteItem) => void
}

export function PaletteList({
  entries,
  label,
  ...props
}: ListProps & { label: string }) {
  const scrollRef = React.useRef<HTMLDivElement>(null)
  const itemCount = entries.filter((e) => e.type === 'item').length
  return (
    <div
      id="command-palette-listbox"
      ref={scrollRef}
      role="listbox"
      aria-label={label}
      className="max-h-80 overflow-y-auto p-2"
    >
      {itemCount > VIRTUALIZE_ABOVE ? (
        <VirtualEntries entries={entries} scrollRef={scrollRef} {...props} />
      ) : (
        entries.map((entry) =>
          entry.type === 'header' ? (
            <Header key={entry.key} label={entry.label} />
          ) : (
            <ResultRow
              key={entry.key}
              item={entry.item}
              optionId={props.optionId(entry.item)}
              active={entry.index === props.activeIndex}
              query={props.query}
              onSelect={() => props.onSelect(entry.item)}
            />
          ),
        )
      )}
    </div>
  )
}
