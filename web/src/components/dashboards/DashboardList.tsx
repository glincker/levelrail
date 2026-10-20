import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { cn } from '@/lib/utils'
import type { CustomDashboard } from '@/types/dashboards'

const VIRTUALIZE_OVER = 50
const ROW_PX = 36

function Row({
  d,
  selected,
  onSelect,
}: {
  d: CustomDashboard
  selected: boolean
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation('dashboards')
  return (
    <button
      type="button"
      aria-current={selected ? 'true' : undefined}
      onClick={() => onSelect(d.id)}
      className={cn(
        'flex h-9 w-full items-center justify-between gap-2 rounded-md px-2.5 text-left text-sm hover:bg-muted',
        selected && 'bg-muted font-medium',
      )}
    >
      <span className="truncate">{d.name}</span>
      <span className="shrink-0 text-xs text-muted-foreground">
        {d.is_default
          ? t('list.default')
          : d.scope === 'shared'
            ? t('list.shared')
            : ''}
      </span>
    </button>
  )
}

/** Names of the visible dashboards; windowed past 50 rows. */
export function DashboardList({
  dashboards,
  selectedId,
  onSelect,
}: {
  dashboards: CustomDashboard[]
  selectedId: string
  onSelect: (id: string) => void
}) {
  const { t } = useTranslation('dashboards')
  const parentRef = useRef<HTMLDivElement>(null)
  const windowed = dashboards.length > VIRTUALIZE_OVER
  const virtualizer = useVirtualizer({
    count: dashboards.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_PX,
    overscan: 8,
    enabled: windowed,
  })

  if (!windowed) {
    return (
      <nav aria-label={t('list.label')} className="space-y-0.5">
        {dashboards.map((d) => (
          <Row
            key={d.id}
            d={d}
            selected={d.id === selectedId}
            onSelect={onSelect}
          />
        ))}
      </nav>
    )
  }
  return (
    <nav aria-label={t('list.label')}>
      <div ref={parentRef} className="max-h-96 overflow-auto">
        <div
          className="relative w-full"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((v) => {
            const d = dashboards[v.index]
            if (!d) return null
            return (
              <div
                key={d.id}
                className="absolute inset-x-0 top-0"
                style={{ transform: `translateY(${v.start}px)` }}
              >
                <Row d={d} selected={d.id === selectedId} onSelect={onSelect} />
              </div>
            )
          })}
        </div>
      </div>
    </nav>
  )
}
