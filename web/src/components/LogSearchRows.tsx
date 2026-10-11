import { useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { LogLevelGutter } from './LogLevelChips'
import { bucketOf } from '../lib/logLevels'
import type { LogEntry } from '../types/logs'

const ROW_HEIGHT_PX = 22

// The virtualized result list of the log search. With `wrap` rows take the
// height of their wrapped text and are measured; otherwise each is one line.
export function LogSearchRows({
  entries,
  showContainer,
  wrap,
  selected,
  onSelect,
}: {
  entries: LogEntry[]
  showContainer: boolean
  wrap: boolean
  selected: number | null
  onSelect: (index: number) => void
}) {
  // State-backed callback ref: useVirtualizer needs a re-render once the
  // scroll container mounts so getScrollElement has something to measure.
  const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null)
  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => scrollEl,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 20,
  })

  return (
    <div
      ref={setScrollEl}
      className="h-[50vh] overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 font-mono text-xs leading-5 text-neutral-200"
    >
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const entry = entries[virtualRow.index]
          if (!entry) {
            return null
          }
          const isSelected = selected === virtualRow.index
          return (
            <div
              key={virtualRow.key}
              data-index={virtualRow.index}
              ref={virtualizer.measureElement}
              role="button"
              tabIndex={0}
              aria-pressed={isSelected}
              onClick={() => {
                onSelect(virtualRow.index)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  onSelect(virtualRow.index)
                }
              }}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: wrap ? undefined : ROW_HEIGHT_PX,
                transform: `translateY(${virtualRow.start}px)`,
              }}
              className={`flex cursor-pointer gap-2 pr-3 hover:bg-neutral-900 ${
                wrap ? 'items-start' : 'items-baseline truncate'
              } ${isSelected ? 'bg-neutral-800' : ''}`}
            >
              <LogLevelGutter
                level={entry.level}
                bucket={bucketOf(entry.level)}
              />
              <span className="shrink-0 text-neutral-500">
                {new Date(entry.timestamp).toLocaleTimeString()}
              </span>
              <span
                className={`shrink-0 rounded px-1 text-[0.65rem] uppercase ${
                  entry.stream === 'stderr'
                    ? 'bg-red-900/40 text-red-300'
                    : 'bg-neutral-800 text-neutral-400'
                }`}
              >
                {entry.stream}
              </span>
              {entry.container && showContainer ? (
                <span className="shrink-0 font-mono text-[0.65rem] text-neutral-500">
                  {entry.container.slice(0, 8)}
                </span>
              ) : null}
              {entry.structured ? (
                <span className="shrink-0 rounded bg-sky-900/40 px-1 text-[0.65rem] uppercase text-sky-300">
                  json
                </span>
              ) : null}
              <span
                className={`${
                  wrap
                    ? 'break-words whitespace-pre-wrap'
                    : 'truncate whitespace-pre'
                } ${
                  entry.stream === 'stderr'
                    ? 'text-red-400'
                    : 'text-neutral-200'
                }`}
              >
                {entry.structured
                  ? formatStructuredFields(entry.fields, entry.message)
                  : entry.message}
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// Structured entries render as compact `key=value` pairs, keeping the row a
// single line for the virtualizer; the detail panel shows the pretty JSON.
function formatStructuredFields(
  fields: Record<string, unknown> | undefined,
  fallbackMessage: string,
): string {
  if (!fields || Object.keys(fields).length === 0) {
    return fallbackMessage
  }
  return Object.entries(fields)
    .map(
      ([key, value]) =>
        `${key}=${typeof value === 'string' ? value : JSON.stringify(value)}`,
    )
    .join(' ')
}
