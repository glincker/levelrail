import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ClockGlassIcon, PlayGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { formatAge } from '../lib/format'
import type { ScheduledTaskRunStatus } from '../types/scheduledTasks'
import { JobStatusBadge } from './JobStatusBadge'

export type ComposeSchedule = {
  id: string
  service: string
  name?: string
  command: string[]
  schedule: string
  enabled: boolean
  last_run_status?: string
  running_since?: string
}

export type ComposeScheduleRun = {
  id: string
  trigger: string
  status: ScheduledTaskRunStatus
  started_at: string
  exit_code?: number
  duration_ms: number
  message?: string
}

const schedulesKey = (appId: string) => ['composeSchedules', appId] as const

function schedulePath(appId: string, s: ComposeSchedule): string {
  return `/api/v1/apps/${encodeURIComponent(appId)}/compose/services/${encodeURIComponent(s.service)}/schedules/${encodeURIComponent(s.id)}`
}

async function getJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, res.statusText))
  }
  return (await res.json()) as T
}

function ScheduleHistory({
  appId,
  schedule,
}: {
  appId: string
  schedule: ComposeSchedule
}) {
  const { t } = useTranslation('compose')
  const runs = useQuery({
    queryKey: [...schedulesKey(appId), schedule.id, 'runs'],
    queryFn: () =>
      getJSON<ComposeScheduleRun[]>(
        `${schedulePath(appId, schedule)}/runs?limit=10`,
      ),
  })
  if (runs.isPending) return null
  if (runs.isError)
    return <p className="text-sm text-destructive">{runs.error.message}</p>
  if (runs.data.length === 0)
    return (
      <p className="text-sm text-muted-foreground">{t('schedules.noRuns')}</p>
    )
  return (
    <ul className="mt-2 space-y-1 text-sm" aria-label={t('schedules.history')}>
      {runs.data.map((run) => (
        <li key={run.id} className="flex flex-wrap items-center gap-2">
          <JobStatusBadge status={run.status} />
          <span>
            {t('schedules.exitCode', {
              code: run.exit_code ?? t('schedules.none'),
            })}
          </span>
          <span className="text-muted-foreground">
            {formatAge(run.started_at)}
          </span>
          {run.message ? (
            <span className="text-muted-foreground">{run.message}</span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

// Lists every member service's schedule for a stack, with run now and the
// last runs; rendered lazily so the stack page's first load stays small.
export function ComposeSchedulesCard({ appId }: { appId: string }) {
  const { t } = useTranslation('compose')
  const queryClient = useQueryClient()
  const [open, setOpen] = useState<string | null>(null)
  const list = useQuery({
    queryKey: schedulesKey(appId),
    queryFn: () =>
      getJSON<{ schedules: ComposeSchedule[] }>(
        `/api/v1/apps/${encodeURIComponent(appId)}/compose/schedules`,
      ),
  })
  const runNow = useMutation({
    mutationFn: (s: ComposeSchedule) =>
      getJSON<{ run_id?: string }>(`${schedulePath(appId, s)}/run`, {
        method: 'POST',
      }),
    onSuccess: (res, s) => {
      toast.add({
        title: t('schedules.started', { id: res.run_id ?? s.id }),
        type: 'success',
      })
      setOpen(s.id)
      void queryClient.invalidateQueries({ queryKey: schedulesKey(appId) })
    },
    onError: (err) => toast.add({ title: err.message, type: 'error' }),
  })

  if (list.isPending || list.isError) return null
  const schedules = list.data.schedules
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ClockGlassIcon className="size-5" aria-hidden="true" />
          {t('schedules.title')}
        </CardTitle>
        <CardDescription>{t('schedules.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {schedules.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('schedules.empty', { app: appId })}
          </p>
        ) : (
          <ul className="divide-y">
            {schedules.map((s) => (
              <li key={s.id} className="py-2">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="info">{s.service}</Badge>
                  <span className="font-medium">{s.name ?? s.id}</span>
                  <code className="text-xs">{s.schedule}</code>
                  <code className="truncate text-xs text-muted-foreground">
                    {s.command.join(' ')}
                  </code>
                  {!s.enabled ? (
                    <Badge variant="muted">{t('schedules.paused')}</Badge>
                  ) : null}
                  <span className="ml-auto flex gap-1">
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      aria-expanded={open === s.id}
                      onClick={() => setOpen(open === s.id ? null : s.id)}
                    >
                      {t('schedules.history')}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={runNow.isPending || Boolean(s.running_since)}
                      aria-label={t('schedules.runNowLabel', {
                        name: s.name ?? s.id,
                        service: s.service,
                      })}
                      onClick={() => runNow.mutate(s)}
                    >
                      <PlayGlassIcon className="size-4" aria-hidden="true" />
                      {t('schedules.runNow')}
                    </Button>
                  </span>
                </div>
                {open === s.id ? (
                  <ScheduleHistory appId={appId} schedule={s} />
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
