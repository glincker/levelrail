import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon, RestoreGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { formatAbsolute } from '../../lib/dateFormat'
import { formatSize } from '../../lib/format'
import { groupRunsByDay, runsExportUrl } from '../../lib/cleanup'
import { useCleanupOverview, useCleanupRuns } from '../../queries/cleanup'
import type { CleanupRun, CleanupRunFilters } from '../../types/cleanup'

const NO_FILTERS: CleanupRunFilters = {
  target: '',
  since: '',
  until: '',
  kind: 'all',
  status: 'all',
}

function useOutcome() {
  const { t } = useTranslation('cleanup')
  return (run: CleanupRun) => {
    const args = {
      count: run.rows,
      rows: run.rows.toLocaleString(),
      size: formatSize(run.bytes),
    }
    if (run.rows === 0) {
      return t(
        run.dry_run ? 'history.outcomeDryNone' : 'history.outcomeRealNone',
      )
    }
    return run.dry_run
      ? t('history.outcomeDry', args)
      : t('history.outcomeReal', args)
  }
}

function RunStatus({ run }: { run: CleanupRun }) {
  const { t } = useTranslation('cleanup')
  if (run.error) {
    return <Badge variant="destructive">{t('history.failed')}</Badge>
  }
  return run.dry_run ? (
    <Badge variant="muted">{t('history.dryRun')}</Badge>
  ) : (
    <Badge variant="success">{t('history.realRun')}</Badge>
  )
}

function FilterBar({
  filters,
  onChange,
}: {
  filters: CleanupRunFilters
  onChange: (next: CleanupRunFilters) => void
}) {
  const { t } = useTranslation('cleanup')
  const overview = useCleanupOverview()
  const targets = overview.data?.targets ?? []
  const set = (patch: Partial<CleanupRunFilters>) => {
    onChange({ ...filters, ...patch })
  }
  const dirty = JSON.stringify(filters) !== JSON.stringify(NO_FILTERS)

  return (
    <div className="flex flex-wrap items-end gap-x-3 gap-y-2">
      <div className="space-y-1">
        <Label htmlFor="cleanup-f-target" className="text-xs">
          {t('history.filters.target')}
        </Label>
        <Select
          value={filters.target}
          onValueChange={(v) => {
            if (v !== null) set({ target: v })
          }}
        >
          <SelectTrigger id="cleanup-f-target" size="sm" className="w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">{t('history.filters.allTargets')}</SelectItem>
            {targets.map((x) => (
              <SelectItem key={x.id} value={x.id}>
                {x.title}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1">
        <Label htmlFor="cleanup-f-since" className="text-xs">
          {t('history.filters.since')}
        </Label>
        <Input
          id="cleanup-f-since"
          type="date"
          className="h-7 w-36"
          value={filters.since}
          onChange={(e) => {
            set({ since: e.target.value })
          }}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="cleanup-f-until" className="text-xs">
          {t('history.filters.until')}
        </Label>
        <Input
          id="cleanup-f-until"
          type="date"
          className="h-7 w-36"
          value={filters.until}
          onChange={(e) => {
            set({ until: e.target.value })
          }}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="cleanup-f-kind" className="text-xs">
          {t('history.filters.kind')}
        </Label>
        <Select
          value={filters.kind}
          onValueChange={(v) => {
            if (v !== null) set({ kind: v })
          }}
        >
          <SelectTrigger id="cleanup-f-kind" size="sm" className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('history.filters.allKinds')}</SelectItem>
            <SelectItem value="dry">{t('history.filters.dry')}</SelectItem>
            <SelectItem value="real">{t('history.filters.real')}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1">
        <Label htmlFor="cleanup-f-status" className="text-xs">
          {t('history.filters.status')}
        </Label>
        <Select
          value={filters.status}
          onValueChange={(v) => {
            if (v !== null) set({ status: v })
          }}
        >
          <SelectTrigger id="cleanup-f-status" size="sm" className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {t('history.filters.allStatuses')}
            </SelectItem>
            <SelectItem value="ok">{t('history.filters.ok')}</SelectItem>
            <SelectItem value="failed">
              {t('history.filters.failed')}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
      {dirty ? (
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            onChange(NO_FILTERS)
          }}
        >
          {t('history.filters.reset')}
        </Button>
      ) : null}
    </div>
  )
}

function RunTimeline({ runs }: { runs: CleanupRun[] }) {
  const { t } = useTranslation('cleanup')
  const outcome = useOutcome()
  const titles = useTargetTitles()
  return (
    <div className="space-y-4 md:hidden" aria-label={t('history.timeline')}>
      {groupRunsByDay(runs).map((d) => (
        <section key={d.day} aria-label={d.day}>
          <h3 className="sticky top-0 z-10 bg-background py-1 text-xs font-medium text-muted-foreground">
            {d.day}
          </h3>
          <ol className="ml-1.5 border-l border-border">
            {d.runs.map((run) => (
              <li key={run.id} className="relative space-y-0.5 py-2 pl-4">
                <span
                  aria-hidden="true"
                  className={cn(
                    'absolute top-3.5 -left-[4.5px] size-2 rounded-full',
                    run.error
                      ? 'bg-tone-danger-solid'
                      : run.dry_run
                        ? 'bg-tone-neutral-solid'
                        : 'bg-tone-success-solid',
                  )}
                />
                <p className="flex flex-wrap items-center gap-1.5 text-sm font-medium text-foreground">
                  {titles(run.target)}
                  <RunStatus run={run} />
                </p>
                <p className="text-sm text-foreground">
                  {run.error ? run.error : outcome(run)}
                </p>
                <p className="text-xs text-muted-foreground">
                  {new Date(run.started_at).toLocaleTimeString()}
                  {' · '}
                  {t('history.meta', {
                    trigger: t(`history.trigger.${run.trigger}`),
                    days: run.days,
                  })}
                  {run.actor
                    ? ` ${t('history.byActor', { actor: run.actor })}`
                    : ''}
                </p>
              </li>
            ))}
          </ol>
        </section>
      ))}
    </div>
  )
}

function useTargetTitles() {
  const overview = useCleanupOverview()
  const targets = overview.data?.targets ?? []
  return (id: string) => targets.find((x) => x.id === id)?.title ?? id
}

function RunTable({ runs }: { runs: CleanupRun[] }) {
  const { t } = useTranslation('cleanup')
  const outcome = useOutcome()
  const titles = useTargetTitles()
  return (
    <div className="hidden md:block">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('history.columns.when')}</TableHead>
            <TableHead>{t('history.columns.target')}</TableHead>
            <TableHead>{t('history.columns.kind')}</TableHead>
            <TableHead>{t('history.columns.result')}</TableHead>
            <TableHead>{t('history.columns.trigger')}</TableHead>
            <TableHead className="text-right">
              {t('history.columns.days')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {runs.map((run) => (
            <TableRow key={run.id}>
              <TableCell className="whitespace-nowrap">
                {formatAbsolute(run.started_at)}
              </TableCell>
              <TableCell>{titles(run.target)}</TableCell>
              <TableCell>
                <RunStatus run={run} />
              </TableCell>
              <TableCell className={cn(run.error && 'text-destructive')}>
                {run.error ? run.error : outcome(run)}
              </TableCell>
              <TableCell>
                {t(`history.trigger.${run.trigger}`)}
                {run.actor ? (
                  <span className="text-muted-foreground">
                    {' '}
                    {t('history.byActor', { actor: run.actor })}
                  </span>
                ) : null}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {run.days}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

export function CleanupHistoryTab() {
  const { t } = useTranslation('cleanup')
  const [filters, setFilters] = useState<CleanupRunFilters>(NO_FILTERS)
  const runs = useCleanupRuns(filters)
  const list = runs.data?.pages.flatMap((pg) => pg.runs) ?? []
  const filtered = JSON.stringify(filters) !== JSON.stringify(NO_FILTERS)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <FilterBar
          filters={filters}
          onChange={(next) => {
            setFilters(next)
          }}
        />
        <div
          role="group"
          aria-label={t('history.exportLabel')}
          className="flex gap-2"
        >
          <a
            href={runsExportUrl(filters, 'csv')}
            download
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            <DownloadGlassIcon size="1.2em" aria-hidden="true" />
            {t('history.exportCsv')}
          </a>
          <a
            href={runsExportUrl(filters, 'json')}
            download
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            <DownloadGlassIcon size="1.2em" aria-hidden="true" />
            {t('history.exportJson')}
          </a>
        </div>
      </div>

      {runs.isPending ? <Skeleton className="h-48 w-full" /> : null}
      {runs.error ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center gap-3">
            {t('history.loadError', { message: runs.error.message })}
            <Button
              size="sm"
              variant="outline"
              onClick={() => void runs.refetch()}
            >
              {t('retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {runs.isSuccess && list.length === 0 ? (
        <EmptyState
          icon={<RestoreGlassIcon className="size-7" />}
          title={t('history.empty.title')}
          description={
            filtered ? t('history.emptyFiltered') : t('history.empty.body')
          }
          action={
            filtered ? (
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setFilters(NO_FILTERS)
                }}
              >
                {t('history.filters.reset')}
              </Button>
            ) : undefined
          }
        />
      ) : null}
      {list.length > 0 ? (
        <>
          <RunTimeline runs={list} />
          <RunTable runs={list} />
          {runs.hasNextPage ? (
            <div className="flex justify-center">
              <Button
                size="sm"
                variant="outline"
                disabled={runs.isFetchingNextPage}
                onClick={() => void runs.fetchNextPage()}
              >
                {runs.isFetchingNextPage
                  ? t('history.loadingMore')
                  : t('history.loadMore')}
              </Button>
            </div>
          ) : null}
        </>
      ) : null}
    </div>
  )
}
