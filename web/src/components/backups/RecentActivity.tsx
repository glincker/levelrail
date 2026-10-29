import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { formatBytes, formatDate } from '../../lib/format'
import { useBackupTargetsOptional } from '../../queries/backupTargets'
import {
  ALL_BACKUP_HISTORY_PAGE_SIZE,
  allBackupHistoryQueryOptions,
  fetchAllBackupHistory,
} from '../../queries/allBackupHistory'
import { RestoreStatusCell } from '../RestoreStatusCell'
import {
  AnyBackupDeleteDialog,
  AnyBackupDownloadLink,
  AnyBackupVerificationBadge,
} from '../AnyBackupActions'
import type { BackupHistoryRecord } from '../../types/backupHistory'

function ResourceLink({ backup }: { backup: BackupHistoryRecord }) {
  if (backup.resource_kind === 'volume') {
    return (
      <Link
        to="/apps/$name/volumes"
        params={{ name: backup.service_name ?? '' }}
        className="text-foreground underline-offset-2 hover:underline"
      >
        {backup.service_name}/{backup.volume_name}
      </Link>
    )
  }
  return (
    <Link
      to="/databases/$name/overview"
      params={{ name: backup.database_name ?? '' }}
      className="text-foreground underline-offset-2 hover:underline"
    >
      {backup.database_name}
    </Link>
  )
}

function ActivityTable() {
  const { t } = useTranslation('backups')
  const first = useQuery(allBackupHistoryQueryOptions())
  const [more, setMore] = useState<BackupHistoryRecord[]>([])
  const [loadingMore, setLoadingMore] = useState(false)
  const [exhausted, setExhausted] = useState(false)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const targets = useBackupTargetsOptional().data ?? []
  const history = [...(first.data ?? []), ...more]
  const targetName = (id: string) =>
    targets.find((x) => x.id === id)?.name ?? t('activity.deletedTarget')

  async function loadMore() {
    const oldest = history[history.length - 1]
    if (!oldest) return
    setLoadingMore(true)
    setLoadMoreError(null)
    try {
      const next = await fetchAllBackupHistory({ before: oldest.started_at })
      setMore((prev) => [...prev, ...next])
      if (next.length < ALL_BACKUP_HISTORY_PAGE_SIZE) setExhausted(true)
    } catch (err) {
      setLoadMoreError(
        err instanceof Error ? err.message : t('activity.loadFailed'),
      )
    } finally {
      setLoadingMore(false)
    }
  }

  const columns: ResponsiveColumn<BackupHistoryRecord>[] = [
    {
      id: 'resource',
      header: t('activity.resource'),
      primary: true,
      cell: (b) => <ResourceLink backup={b} />,
    },
    {
      id: 'status',
      header: t('activity.status'),
      cell: (b) => <RestoreStatusCell status={b.status} error={b.error} />,
    },
    {
      id: 'kind',
      header: t('activity.kind'),
      priority: 2,
      cell: (b) => (
        <Badge variant="outline">
          {t(
            `row.kind.${b.resource_kind === 'volume' ? 'volume' : 'database'}`,
          )}
        </Badge>
      ),
    },
    {
      id: 'target',
      header: t('activity.target'),
      priority: 2,
      cell: (b) => targetName(b.target_id),
    },
    {
      id: 'size',
      header: t('activity.size'),
      priority: 3,
      cell: (b) => (
        <span className="text-muted-foreground">
          {b.status === 'running' ? '-' : formatBytes(b.size_bytes)}
        </span>
      ),
    },
    {
      id: 'started',
      header: t('activity.started'),
      priority: 3,
      cell: (b) => (
        <span className="text-muted-foreground">
          {formatDate(b.started_at, '-')}
        </span>
      ),
    },
    {
      id: 'verification',
      header: t('activity.verification'),
      priority: 3,
      cell: (b) =>
        b.status === 'succeeded' ? (
          <AnyBackupVerificationBadge backup={b} />
        ) : (
          <span className="text-muted-foreground">-</span>
        ),
    },
  ]

  if (first.isPending) return <TableSkeleton columnCount={8} rowCount={4} />
  if (first.isError) {
    return <p className="text-sm text-destructive">{first.error.message}</p>
  }
  if (history.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('activity.empty')}</p>
    )
  }
  const canLoadMore =
    !exhausted && first.data.length >= ALL_BACKUP_HISTORY_PAGE_SIZE

  return (
    <div className="space-y-3">
      <ResponsiveTable
        rows={history}
        columns={columns}
        getRowId={(b) => b.id}
        ariaLabel={t('tables.activity')}
        renderActions={(b) => (
          <div className="flex items-center gap-2">
            {b.status === 'succeeded' ? (
              <AnyBackupDownloadLink backup={b} />
            ) : null}
            {b.status !== 'running' ? (
              <AnyBackupDeleteDialog backup={b} />
            ) : null}
          </div>
        )}
      />
      {loadMoreError ? (
        <p className="text-sm text-destructive">{loadMoreError}</p>
      ) : null}
      {canLoadMore ? (
        <Button
          type="button"
          variant="outline"
          disabled={loadingMore}
          onClick={() => {
            void loadMore()
          }}
        >
          {loadingMore ? t('activity.loading') : t('activity.loadOlder')}
        </Button>
      ) : null}
    </div>
  )
}

/** RecentActivity is the every-attempt history, collapsed until asked for. */
export function RecentActivity() {
  const { t } = useTranslation('backups')
  const [open, setOpen] = useState(false)
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex items-center gap-2 rounded-lg border border-border px-4 py-3">
        <ChevronDownGlassIcon
          className={`size-5 transition-transform ${open ? '' : '-rotate-90'}`}
          aria-hidden="true"
        />
        <span className="text-base font-medium">{t('activity.title')}</span>
        <span className="text-sm text-muted-foreground">
          {t('activity.subtitle')}
        </span>
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="pt-3">{open ? <ActivityTable /> : null}</div>
      </CollapsiblePanel>
    </Collapsible>
  )
}
