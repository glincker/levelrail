import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RestoreGlassIcon } from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CodeBlock } from '@/components/ui/code-block'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { useJobRun, useJobRuns } from '../queries/scheduledTasks'
import type { JobRun, ScheduledTaskRunStatus } from '../types/scheduledTasks'
import { JobStatusBadge } from './JobStatusBadge'

const STATUSES: ScheduledTaskRunStatus[] = [
  'running',
  'success',
  'failed',
  'timeout',
  'missed',
  'interrupted',
  'container_not_running',
  'skipped_concurrency',
  'replaced',
]

const ALL = 'all'

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)} s`
  return `${Math.floor(s / 60)} m ${Math.round(s % 60)} s`
}

const BAR_WIDTH = 8
const BAR_GAP = 3
const TREND_HEIGHT = 40

const BAR_CLASS: Partial<Record<ScheduledTaskRunStatus, string>> = {
  success: 'fill-primary',
  failed: 'fill-destructive',
  timeout: 'fill-destructive',
  interrupted: 'fill-destructive',
}

// DurationTrend draws one bar per finished run, oldest to newest, scaled to
// the slowest run in view, so a slowdown after a deploy shows as a step.
function DurationTrend({ runs }: { runs: JobRun[] }) {
  const { t } = useTranslation('jobs')
  const finished = runs
    .filter((r) => r.status !== 'running' && r.duration_ms > 0)
    .slice()
    .reverse()
  if (finished.length < 2) return null
  const max = Math.max(...finished.map((r) => r.duration_ms))
  const width = finished.length * (BAR_WIDTH + BAR_GAP)
  return (
    <figure className="mt-3">
      <svg
        role="img"
        aria-label={t('history.trendLabel', { count: finished.length })}
        viewBox={`0 0 ${width} ${TREND_HEIGHT}`}
        className="h-10 w-full max-w-md"
        preserveAspectRatio="none"
      >
        {finished.map((r, i) => {
          const h = Math.max(2, (r.duration_ms / max) * TREND_HEIGHT)
          return (
            <rect
              key={r.id}
              x={i * (BAR_WIDTH + BAR_GAP)}
              y={TREND_HEIGHT - h}
              width={BAR_WIDTH}
              height={h}
              className={BAR_CLASS[r.status] ?? 'fill-muted-foreground'}
            >
              <title>{`${new Date(r.started_at).toLocaleString()}: ${formatDuration(r.duration_ms)}`}</title>
            </rect>
          )
        })}
      </svg>
      <figcaption className="text-xs text-muted-foreground">
        {t('history.trendCaption', { max: formatDuration(max) })}
      </figcaption>
    </figure>
  )
}

function RunLogsDialog({ appName, run }: { appName: string; run: JobRun }) {
  const { t } = useTranslation('jobs')
  const [open, setOpen] = useState(false)
  const { data, isLoading } = useJobRun(
    appName,
    open ? run.task_id : '',
    run.id,
  )
  const output = data?.output ?? ''

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        {t('history.viewLogs')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('history.logsTitle')}</DialogTitle>
          <DialogDescription>
            {run.job_name || run.task_id} {'· '}
            {new Date(run.started_at).toLocaleString()}
            {' · '}
            {t('history.logsRedacted')}
          </DialogDescription>
        </DialogHeader>
        {data?.message ? (
          <p className="text-sm text-muted-foreground">{data.message}</p>
        ) : null}
        {isLoading ? (
          <p className="text-sm text-muted-foreground">
            {t('history.logsLoading')}
          </p>
        ) : output ? (
          <CodeBlock code={output} wrap maxHeight="lg" />
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('history.logsEmpty')}
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}

// JobRunsPanel is the app-wide run history: newest first, filterable by
// status, with a per-run log view. It asks for at most JOB_RUNS_PAGE rows,
// so it never needs virtualizing.
export function JobRunsPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('jobs')
  const [status, setStatus] = useState<string>(ALL)
  const { data, isLoading, error } = useJobRuns(
    appName,
    '',
    status === ALL ? '' : status,
  )
  const runs = data ?? []

  const columns: ResponsiveColumn<JobRun>[] = [
    {
      id: 'job',
      header: t('history.columns.job'),
      primary: true,
      cell: (run) => (
        <span className="text-xs">{run.job_name || run.task_id}</span>
      ),
    },
    {
      id: 'status',
      header: t('history.columns.status'),
      cell: (run) => <JobStatusBadge status={run.status} />,
    },
    {
      id: 'started',
      header: t('history.columns.started'),
      priority: 2,
      cell: (run) => (
        <span className="text-xs text-muted-foreground">
          {new Date(run.started_at).toLocaleString()}
        </span>
      ),
    },
    {
      id: 'trigger',
      header: t('history.columns.trigger'),
      priority: 2,
      cell: (run) => (
        <Badge variant="muted">{t(`trigger.${run.trigger}`)}</Badge>
      ),
    },
    {
      id: 'duration',
      header: t('history.columns.duration'),
      priority: 3,
      cell: (run) => (
        <span className="text-xs">{formatDuration(run.duration_ms)}</span>
      ),
    },
    {
      id: 'exit',
      header: t('history.columns.exit'),
      priority: 3,
      cell: (run) => (
        <span className="font-mono text-xs">
          {run.exit_code === undefined
            ? '-'
            : run.mode === 'http'
              ? t('history.httpStatus', { code: run.exit_code })
              : run.exit_code}
        </span>
      ),
    },
    {
      id: 'attempts',
      header: t('history.columns.attempts'),
      priority: 3,
      cell: (run) => <span className="text-xs">{run.attempts}</span>,
    },
    {
      id: 'node',
      header: t('history.columns.node'),
      priority: 3,
      cell: (run) => (
        <span className="text-xs text-muted-foreground">
          {run.node_id || t('localNode')}
        </span>
      ),
    },
  ]

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            <RestoreGlassIcon
              className="size-4.5 text-muted-foreground"
              aria-hidden="true"
            />
            {t('history.title')}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('history.description')}
          </p>
        </div>
        <Select<string>
          value={status}
          onValueChange={(value) => {
            setStatus(value ?? ALL)
          }}
        >
          <SelectTrigger className="w-52" aria-label={t('history.filter')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t('history.filterAll')}</SelectItem>
            {STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {t(`status.${s}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <DurationTrend runs={runs} />

      <div className="mt-3">
        {isLoading ? (
          <TableSkeleton columnCount={8} rowCount={3} />
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : runs.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {status === ALL ? t('history.empty') : t('history.emptyFiltered')}
          </p>
        ) : (
          <ResponsiveTable
            rows={runs}
            columns={columns}
            getRowId={(run) => run.id}
            ariaLabel={t('history.title')}
            renderActions={(run) => (
              <RunLogsDialog appName={appName} run={run} />
            )}
          />
        )}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        {t('history.alertsHint')}
      </p>
    </section>
  )
}
