import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftGlassIcon,
  ArrowRightGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  moveWidget,
  removeWidget,
  resizeWidget,
  SIZE_SPAN,
  widgetTitle,
  WIDGET_SIZES,
} from '@/lib/dashboardGrid'
import { cn } from '@/lib/utils'
import type { DashboardWidget } from '@/types/dashboards'

/** A responsive four column grid; edit mode adds keyboard reachable move, resize and remove buttons. */
export function DashboardGrid({
  widgets,
  editing,
  onChange,
  renderBody,
}: {
  widgets: DashboardWidget[]
  editing: boolean
  onChange: (next: DashboardWidget[]) => void
  renderBody: (w: DashboardWidget) => ReactNode
}) {
  const { t } = useTranslation('dashboards')
  return (
    <ul
      className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4"
      aria-label={t('grid.label')}
    >
      {widgets.map((w, i) => {
        const title = widgetTitle(w, t(`widget.type.${w.type}`))
        return (
          <li
            key={w.id}
            className={cn(
              'flex min-w-0 flex-col rounded-lg border border-border bg-card p-4 text-card-foreground',
              w.size === 'S' ? 'md:col-span-1' : 'md:col-span-2',
              SIZE_SPAN[w.size],
            )}
          >
            <div className="mb-3 flex items-start justify-between gap-2">
              <h3 className="truncate text-sm font-medium">{title}</h3>
              {editing ? (
                <div
                  className="flex shrink-0 items-center gap-1"
                  role="group"
                  aria-label={t('grid.controls', { title })}
                >
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    disabled={i === 0}
                    aria-label={t('grid.moveEarlier', { title })}
                    onClick={() => onChange(moveWidget(widgets, w.id, -1))}
                  >
                    <ArrowLeftGlassIcon size="1.4em" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    disabled={i === widgets.length - 1}
                    aria-label={t('grid.moveLater', { title })}
                    onClick={() => onChange(moveWidget(widgets, w.id, 1))}
                  >
                    <ArrowRightGlassIcon size="1.4em" />
                  </Button>
                  {WIDGET_SIZES.map((size) => (
                    <Button
                      key={size}
                      type="button"
                      variant={w.size === size ? 'secondary' : 'ghost'}
                      size="icon-sm"
                      aria-pressed={w.size === size}
                      aria-label={t('grid.size', { title, size })}
                      onClick={() =>
                        onChange(resizeWidget(widgets, w.id, size))
                      }
                    >
                      {size}
                    </Button>
                  ))}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('grid.remove', { title })}
                    onClick={() => onChange(removeWidget(widgets, w.id))}
                  >
                    <TrashGlassIcon size="1.4em" />
                  </Button>
                </div>
              ) : null}
            </div>
            <div className="min-w-0 flex-1">{renderBody(w)}</div>
          </li>
        )
      })}
    </ul>
  )
}
