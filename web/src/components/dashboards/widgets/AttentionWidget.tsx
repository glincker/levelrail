import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  InfoGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import type { AttentionSeverity } from '@/lib/attention'
import { useAttentionItems } from '@/queries/attention'
import { WidgetSkeleton } from './parts'

const MAX_ITEMS = 4

const SEVERITY = {
  critical: { Icon: CircleAlertGlassIcon, cls: 'text-tone-danger' },
  warning: { Icon: TriangleAlertGlassIcon, cls: 'text-tone-warning' },
  info: { Icon: InfoGlassIcon, cls: 'text-tone-info' },
} as const satisfies Record<AttentionSeverity, unknown>

/** Open issues from the Status page, worst first. */
export function AttentionWidget() {
  const { t } = useTranslation('dashboards')
  const { items, isLoading } = useAttentionItems()
  if (isLoading) return <WidgetSkeleton />
  if (items.length === 0) {
    return (
      <p className="flex items-center gap-2 text-sm text-tone-success">
        <CircleCheckGlassIcon aria-hidden="true" className="size-5" />
        {t('fleet.attention.clear')}
      </p>
    )
  }
  return (
    <div className="space-y-2">
      <ul className="space-y-1.5 text-sm">
        {items.slice(0, MAX_ITEMS).map((i) => {
          const { Icon, cls } = SEVERITY[i.severity]
          return (
            <li key={i.id} className="flex items-start gap-2">
              <Icon
                aria-hidden="true"
                className={`mt-0.5 size-4 shrink-0 ${cls}`}
              />
              <span className="sr-only">
                {t(`fleet.attention.${i.severity}`)}
              </span>
              <span className="min-w-0 truncate">{i.title}</span>
            </li>
          )
        })}
      </ul>
      <Link
        to="/status"
        className="text-xs text-muted-foreground hover:underline"
      >
        {items.length > MAX_ITEMS
          ? t('fleet.attention.more', { count: items.length - MAX_ITEMS })
          : t('fleet.attention.viewAll')}
      </Link>
    </div>
  )
}
