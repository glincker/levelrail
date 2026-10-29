import { useTranslation } from 'react-i18next'
import { RestoreGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { RelativeTime } from '@/components/kit/RelativeTime'
import { useCachePurges, type PurgeRecord } from '../../queries/cdnCache'
import { formatCount } from './format'

const COLUMN_COUNT = 5

/** PurgeHistoryTable lists who purged what from an app's cache, newest first. */
export function PurgeHistoryTable({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('cdn')
  const { data, error, isPending } = useCachePurges(app)

  if (isPending)
    return <TableSkeleton columnCount={COLUMN_COUNT} rowCount={4} />
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{error.message}</AlertDescription>
      </Alert>
    )
  }
  if (data.length === 0) {
    return (
      <EmptyState
        compact
        icon={<RestoreGlassIcon className="size-4.5" />}
        title={t('history.emptyTitle')}
        description={t('history.emptyBody')}
      />
    )
  }

  const columns: ResponsiveColumn<PurgeRecord>[] = [
    {
      id: 'when',
      header: t('history.when'),
      primary: true,
      cell: (r) => <RelativeTime at={r.at} />,
    },
    { id: 'who', header: t('history.who'), cell: (r) => r.actor },
    {
      id: 'domain',
      header: t('history.domain'),
      priority: 2,
      cell: (r) => (
        <span
          className="block max-w-56 truncate font-mono text-xs"
          title={r.domain}
        >
          {r.domain}
        </span>
      ),
    },
    {
      id: 'scope',
      header: t('history.scope'),
      cell: (r) => (
        <span className="flex min-w-0 items-center gap-2">
          <Badge variant={r.scope === 'all' ? 'warning' : 'muted'}>
            {t(`purge.scope.${r.scope}.title`)}
          </Badge>
          {r.value ? (
            <span
              className="min-w-0 truncate font-mono text-xs text-muted-foreground"
              title={r.value}
            >
              {r.value}
            </span>
          ) : null}
        </span>
      ),
    },
    {
      id: 'removed',
      header: t('history.removed'),
      align: 'end',
      cell: (r) => (
        <span className="tabular-nums">{formatCount(r.removed)}</span>
      ),
    },
  ]

  return (
    <ResponsiveTable
      rows={data}
      columns={columns}
      getRowId={(r) => `${r.at}|${r.domain}|${r.scope}|${r.value ?? ''}`}
      ariaLabel={t('history.aria', { app })}
    />
  )
}
