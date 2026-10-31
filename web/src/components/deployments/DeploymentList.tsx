import { useMemo, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CommitGraphGlassIcon,
  GitBranchGlassIcon,
  RefreshCwGlassIcon,
  UndoGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { RelativeTime, SkeletonLine } from '@/components/kit'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { cn } from '@/lib/utils'
import type { Deployment } from '../../types/deployment'
import {
  headline,
  isRollback,
  rowKind,
  shortSha,
  subtitle,
} from '../../lib/deploymentPresentation'
import { Author, EnvPill, RowThumb, StatusCell } from './DeploymentRow'

export const VIRTUALIZE_AFTER = 50

export function DeploymentsSkeleton({ rows = 8 }: { rows?: number }) {
  return (
    <div
      role="status"
      aria-label="Loading deployments"
      className="overflow-hidden rounded-xl border border-border bg-card"
    >
      {Array.from({ length: rows }, (_, i) => (
        <div
          key={i}
          data-testid="deployment-skeleton-row"
          className="flex items-center gap-3 border-b border-border px-3 py-3.5 last:border-b-0"
        >
          <SkeletonLine width="38%" />
          <div className="ml-auto flex items-center gap-4">
            <SkeletonLine width={64} />
            <SkeletonLine width={80} className="hidden md:block" />
            <SkeletonLine width={56} className="hidden md:block" />
          </div>
        </div>
      ))}
    </div>
  )
}

export interface DeploymentListProps {
  rows: Deployment[]
  now: number
  openId: string
  focusId: string
  onOpen: (id: string) => void
  scrollRef: RefObject<HTMLDivElement | null>
}

function TitleCell({ d }: { d: Deployment }) {
  const kind = rowKind(d)
  const sub = subtitle(d)
  const Lead = isRollback(d)
    ? UndoGlassIcon
    : kind === 'redeploy'
      ? RefreshCwGlassIcon
      : null
  return (
    <span className="flex min-w-0 flex-col">
      <span className="flex min-w-0 items-center gap-1.5 text-sm font-medium">
        {Lead ? (
          <Lead
            className="size-3.5 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        ) : null}
        <span className="truncate" title={headline(d)}>
          {headline(d)}
        </span>
      </span>
      {sub ? (
        <span
          className={cn(
            'truncate text-xs',
            d.status === 'failed'
              ? 'text-tone-danger'
              : 'text-muted-foreground',
          )}
          title={sub}
        >
          {sub}
        </span>
      ) : null}
    </span>
  )
}

export function DeploymentList({
  rows,
  now,
  openId,
  focusId,
  onOpen,
  scrollRef,
}: DeploymentListProps) {
  const { t } = useTranslation('deploys')
  const columns = useMemo<ResponsiveColumn<Deployment>[]>(
    () => [
      {
        id: 'deploy',
        primary: true,
        header: t('table.deploy'),
        cell: (d) => <TitleCell d={d} />,
      },
      {
        id: 'status',
        header: t('table.status'),
        width: 'w-44',
        cell: (d) => <StatusCell d={d} now={now} />,
      },
      {
        id: 'app',
        header: t('table.app'),
        priority: 2,
        width: 'w-36',
        cell: (d) => (
          <span
            className="block truncate text-sm text-foreground"
            title={d.app}
          >
            {d.app}
          </span>
        ),
      },
      {
        id: 'env',
        header: t('table.environment'),
        priority: 3,
        width: 'w-28',
        cell: (d) => <EnvPill d={d} />,
      },
      {
        id: 'commit',
        header: t('table.commit'),
        priority: 3,
        width: 'w-28',
        cell: (d) =>
          !isRollback(d) && d.commit_sha ? (
            <span className="flex items-center gap-1 font-mono text-xs text-muted-foreground">
              <CommitGraphGlassIcon className="size-5" aria-hidden="true" />
              {shortSha(d.commit_sha)}
            </span>
          ) : null,
      },
      {
        id: 'branch',
        header: t('table.branch'),
        priority: 3,
        width: 'w-32',
        cell: (d) =>
          !isRollback(d) && d.branch ? (
            <span className="flex items-center gap-1 font-mono text-xs text-muted-foreground">
              <GitBranchGlassIcon
                className="size-5 shrink-0"
                aria-hidden="true"
              />
              <span className="truncate">{d.branch}</span>
            </span>
          ) : null,
      },
      {
        id: 'started',
        header: t('table.started'),
        priority: 2,
        width: 'w-28',
        cell: (d) => (
          <span className="text-xs text-muted-foreground">
            <RelativeTime at={d.started_at} live />
          </span>
        ),
      },
      {
        id: 'by',
        header: t('table.by'),
        priority: 3,
        width: 'w-24',
        hideInCard: true,
        cell: (d) => (
          <span className="flex items-center gap-2">
            <RowThumb url={d.preview_image_url} app={d.app} />
            <Author name={d.author} />
          </span>
        ),
      },
    ],
    [t, now],
  )
  // Columns with no value on any row are dropped so the headline gets the space.
  const visibleColumns = useMemo(() => {
    const filled: Record<string, boolean> = {
      env: rows.some((d) => Boolean(d.environment)),
      commit: rows.some((d) => !isRollback(d) && Boolean(d.commit_sha)),
      branch: rows.some((d) => !isRollback(d) && Boolean(d.branch)),
      by: rows.some((d) => Boolean(d.author) || d.preview_image_url !== null),
    }
    return columns.filter((c) => filled[c.id] !== false)
  }, [columns, rows])
  return (
    <ResponsiveTable
      rows={rows}
      columns={visibleColumns}
      getRowId={(d) => d.id}
      ariaLabel={t('table.label')}
      virtualizeOver={VIRTUALIZE_AFTER}
      scrollHeightClass="h-[70vh]"
      scrollRef={scrollRef}
      focusRowId={focusId}
      onRowClick={(d) => {
        onOpen(d.id)
      }}
      getRowProps={(d) => ({
        'data-kind': rowKind(d),
        'aria-current': d.id === openId ? 'true' : undefined,
        className: cn(
          d.id === openId && 'bg-muted/60',
          d.id === focusId && 'ring-2 ring-ring/60 ring-inset',
          (rowKind(d) === 'superseded' || d.status === 'canceled') &&
            'opacity-70',
        ),
      })}
    />
  )
}

export function LoadMore({
  hasNext,
  fetching,
  failed,
  onLoad,
}: {
  hasNext: boolean
  fetching: boolean
  failed: boolean
  onLoad: () => void
}) {
  if (!hasNext) return null
  return (
    <div className="flex flex-col items-center gap-1 pt-3">
      {failed && (
        <p className="text-xs text-tone-danger">Could not load more.</p>
      )}
      <Button variant="outline" size="sm" disabled={fetching} onClick={onLoad}>
        {fetching ? 'Loading' : failed ? 'Retry' : 'Load More'}
      </Button>
    </div>
  )
}
