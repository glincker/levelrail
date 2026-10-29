import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { EllipsisGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ActionMenu, type ActionMenuItem } from '@/components/kit/ActionMenu'
import { Sparkline } from '@/components/kit'
import { cn } from '@/lib/utils'
import type { FwTopRow } from '../../queries/firewallEdge'
import { formatCount } from './firewallFormat'

const ROW =
  'grid grid-cols-[1.25rem_minmax(0,1fr)_auto_4rem_2rem] items-center gap-2 px-2 py-1.5 text-sm'

/** TopList ranks keys by count with a sparkline and a keyboard reachable row menu. */
export function TopList({
  title,
  rows,
  mono = false,
  empty,
  menuFor,
  sparkLabel,
  action,
  className,
}: {
  title: string
  rows: FwTopRow[]
  /** Keys are addresses or paths; shown in the monospace face. */
  mono?: boolean
  /** Explains why the list is empty and what to do next. */
  empty: ReactNode
  menuFor?: (row: FwTopRow) => ActionMenuItem[]
  sparkLabel: (row: FwTopRow) => string
  action?: ReactNode
  className?: string
}) {
  const { t } = useTranslation('firewall')
  return (
    <Card className={cn('min-w-0', className)}>
      <CardHeader className="border-b">
        <CardTitle>{title}</CardTitle>
        {action ? <div data-slot="card-action">{action}</div> : null}
      </CardHeader>
      <CardContent className="px-2">
        {rows.length === 0 ? (
          <div className="px-2 py-6 text-center text-sm text-muted-foreground">
            {empty}
          </div>
        ) : (
          <ol className="divide-y divide-border">
            {rows.map((row, i) => (
              <li key={row.key} className={ROW}>
                <span className="text-xs text-muted-foreground tabular-nums">
                  {i + 1}
                </span>
                <span
                  title={row.key}
                  className={cn('truncate', mono && 'font-mono text-[13px]')}
                >
                  {row.key}
                </span>
                <span className="text-right font-medium tabular-nums">
                  {formatCount(row.count)}
                </span>
                <Sparkline
                  values={row.sparkline}
                  width={64}
                  height={20}
                  ariaLabel={sparkLabel(row)}
                />
                {menuFor ? (
                  <ActionMenu
                    trigger={
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t('topList.actionsFor', { key: row.key })}
                      >
                        <EllipsisGlassIcon size="1.2em" />
                      </Button>
                    }
                    items={menuFor(row)}
                  />
                ) : (
                  <span />
                )}
              </li>
            ))}
          </ol>
        )}
      </CardContent>
    </Card>
  )
}

export function TopListSkeleton({
  rows = 5,
  className,
}: {
  rows?: number
  className?: string
}) {
  return (
    <Card aria-hidden="true" className={cn('min-w-0', className)}>
      <CardHeader className="border-b">
        <Skeleton className="h-4 w-28" />
      </CardHeader>
      <CardContent className="px-2">
        {Array.from({ length: rows }, (_, i) => (
          <div key={i} className={ROW}>
            <Skeleton className="h-3 w-3" />
            <Skeleton className="h-4 w-3/4" />
            <Skeleton className="h-4 w-8" />
            <Skeleton className="h-4 w-16" />
            <span />
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
