import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  EyeOffGlassIcon,
  GripVerticalGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { PINNED_VISIBLE, type WidgetId } from '../../lib/dashboardLayout'

const DRAG_TYPE = 'application/x-dashboard-widget'

/** WidgetFrame adds move and hide controls around a widget while the dashboard is in customize mode. */
export function WidgetFrame({
  id,
  index,
  total,
  editing,
  onMove,
  onHide,
  children,
}: {
  id: WidgetId
  index: number
  total: number
  editing: boolean
  onMove: (id: WidgetId, to: number) => void
  onHide: (id: WidgetId) => void
  children: ReactNode
}) {
  const { t } = useTranslation('dashboard')
  if (!editing) return <>{children}</>
  const name = t(`widgets.${id}`)
  return (
    <div
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes(DRAG_TYPE)) e.preventDefault()
      }}
      onDrop={(e) => {
        const from = e.dataTransfer.getData(DRAG_TYPE) as WidgetId
        if (from) onMove(from, index)
      }}
      className="space-y-2 rounded-xl border border-dashed border-border p-2"
    >
      <div className="flex items-center gap-1 text-xs text-muted-foreground">
        <span
          draggable
          aria-hidden="true"
          onDragStart={(e) => {
            e.dataTransfer.setData(DRAG_TYPE, id)
            e.dataTransfer.effectAllowed = 'move'
          }}
          className="cursor-grab rounded p-1 hover:bg-muted active:cursor-grabbing"
        >
          <GripVerticalGlassIcon className="size-5" />
        </span>
        <span className="flex-1 font-medium text-foreground">{name}</span>
        <button
          type="button"
          aria-label={t('customize.moveUp', { name })}
          disabled={index === 0}
          onClick={() => {
            onMove(id, index - 1)
          }}
          className="rounded p-1 hover:bg-muted disabled:opacity-40"
        >
          <ArrowUpGlassIcon className="size-5" />
        </button>
        <button
          type="button"
          aria-label={t('customize.moveDown', { name })}
          disabled={index === total - 1}
          onClick={() => {
            onMove(id, index + 1)
          }}
          className="rounded p-1 hover:bg-muted disabled:opacity-40"
        >
          <ArrowDownGlassIcon className="size-5" />
        </button>
        <button
          type="button"
          aria-label={t('customize.hide', { name })}
          disabled={PINNED_VISIBLE.includes(id)}
          onClick={() => {
            onHide(id)
          }}
          className={cn('rounded p-1 hover:bg-muted disabled:opacity-40')}
        >
          <EyeOffGlassIcon className="size-5" />
        </button>
      </div>
      {children}
    </div>
  )
}
