import { useEffect, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import type { TemplateDensity } from '../../hooks/useTemplateDensity'
import type { ServiceTemplateListItem } from '../../queries/serviceTemplates'
import { TemplateCard } from './TemplateCard'

// Card min-width drives the breakpoints so a logo, name and button never wrap.
function columnsForWidth(width: number): number {
  if (width >= 1360) return 4
  if (width >= 1024) return 3
  if (width >= 640) return 2
  return 1
}

const ROW_ESTIMATE: Record<TemplateDensity, number> = {
  comfortable: 232,
  compact: 72,
}

// Module scope so the offset survives the route unmounting on a detail visit.
let savedOffset = 0

function useGridColumns(
  ref: React.RefObject<HTMLDivElement | null>,
  density: TemplateDensity,
): number {
  const [cols, setCols] = useState(1)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return undefined
    const ro = new ResizeObserver(([entry]) => {
      if (entry) setCols(columnsForWidth(entry.contentRect.width))
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
    }
  }, [ref])
  return density === 'compact' ? 1 : cols
}

/** TemplateGrid virtualizes the catalog as rows of cards or compact list rows. */
export function TemplateGrid({
  templates,
  density,
  resetKey,
  onSelect,
  onDeployNow,
  deployingId,
  customTemplateIds,
}: {
  templates: ServiceTemplateListItem[]
  density: TemplateDensity
  resetKey: string
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deployingId?: string
  customTemplateIds: Set<string>
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const cols = useGridColumns(parentRef, density)
  const compact = density === 'compact'
  const rowCount = Math.ceil(templates.length / cols)
  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_ESTIMATE[density],
    overscan: 4,
    initialOffset: savedOffset,
    onChange: (instance) => {
      savedOffset = instance.scrollOffset ?? 0
    },
  })

  const firstRun = useRef(true)
  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false
      return
    }
    savedOffset = 0
    virtualizer.scrollToOffset(0)
    // Only a changed filter/sort/density jumps to the top, not every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, density])

  return (
    <div ref={parentRef} className="h-[70vh] overflow-auto pr-1">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const slice = templates.slice(
            virtualRow.index * cols,
            virtualRow.index * cols + cols,
          )
          return (
            <div
              key={virtualRow.key}
              data-index={virtualRow.index}
              ref={virtualizer.measureElement}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              <div
                className={compact ? 'grid gap-2 pb-2' : 'grid gap-4 pb-4'}
                style={{
                  gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
                }}
              >
                {slice.map((template) => (
                  <TemplateCard
                    key={template.id}
                    template={template}
                    compact={compact}
                    onSelect={onSelect}
                    onDeployNow={onDeployNow}
                    isCustom={customTemplateIds.has(template.id)}
                    deploying={deployingId === template.id}
                  />
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
