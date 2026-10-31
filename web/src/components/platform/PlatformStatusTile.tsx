import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { PlatformPageId } from '../../lib/platformSettingsIndex'
import { PLATFORM_PAGE_PATH } from '../../lib/platformSettingsIndex'

export type TileTone = 'success' | 'warning' | 'muted'

export interface PlatformStatusTileProps {
  id: PlatformPageId
  icon: ReactNode
  title: string
  /** Undefined while the status is still loading. */
  chip?: { label: string; tone: TileTone }
  line?: string
  action?: ReactNode
}

export function PlatformStatusTile({
  id,
  icon,
  title,
  chip,
  line,
  action,
}: PlatformStatusTileProps) {
  const { t } = useTranslation('platformSettings')
  const headingId = `platform-tile-${id}`
  return (
    <li className="list-none">
      <article
        aria-labelledby={headingId}
        className="flex h-full flex-col gap-2 rounded-lg border border-border bg-card p-4"
      >
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground" aria-hidden="true">
            {icon}
          </span>
          <h2
            id={headingId}
            className="min-w-0 flex-1 text-sm font-semibold text-foreground"
          >
            {title}
          </h2>
          {chip ? (
            <Badge variant={chip.tone}>{chip.label}</Badge>
          ) : (
            <Skeleton className="h-5 w-24" />
          )}
        </div>
        {line === undefined ? (
          <Skeleton className="h-4 w-3/4" />
        ) : (
          <p className="min-w-0 break-words text-sm text-muted-foreground">
            {line}
          </p>
        )}
        <div className="mt-auto flex items-center justify-between gap-2 pt-1">
          <span>{action}</span>
          <Link
            to={PLATFORM_PAGE_PATH[id]}
            aria-label={`${t('overview.open')} ${title}`}
            className="inline-flex items-center gap-1 text-sm text-foreground hover:underline"
          >
            {t('overview.open')}
            <ChevronRightGlassIcon size="1.2em" aria-hidden="true" />
          </Link>
        </div>
      </article>
    </li>
  )
}
