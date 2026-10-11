import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ClockCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-xs text-foreground">
            {output}
          </pre>
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

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <ClockCounterClockwiseIcon
              className="size-4 text-muted-foreground"
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
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('history.columns.started')}</TableHead>
                  <TableHead>{t('history.columns.job')}</TableHead>
                  <TableHead>{t('history.columns.status')}</TableHead>
                  <TableHead>{t('history.columns.trigger')}</TableHead>
                  <TableHead>{t('history.columns.duration')}</TableHead>
                  <TableHead>{t('history.columns.exit')}</TableHead>
                  <TableHead>{t('history.columns.attempts')}</TableHead>
                  <TableHead>{t('history.columns.node')}</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((run) => (
                  <TableRow key={run.id}>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(run.started_at).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-xs">
                      {run.job_name || run.task_id}
                    </TableCell>
                    <TableCell>
                      <JobStatusBadge status={run.status} />
                    </TableCell>
                    <TableCell>
                      <Badge variant="muted">
                        {t(`trigger.${run.trigger}`)}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs">
                      {formatDuration(run.duration_ms)}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {run.exit_code ?? '-'}
                    </TableCell>
                    <TableCell className="text-xs">{run.attempts}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {run.node_id || t('localNode')}
                    </TableCell>
                    <TableCell className="text-right">
                      <RunLogsDialog appName={appName} run={run} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        {t('history.alertsHint')}
      </p>
    </section>
  )
}
