import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CountdownGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'
import { InfoTip } from './kit'
import { formatDate } from '../lib/format'
import { formatRelativeTime } from '../lib/relativeTime'
import {
  useAppSchedule,
  useAppScheduleHistory,
  useSetAppSchedule,
} from '../queries/appSchedule'

type Mode = 'cron' | 'once'

interface Draft {
  mode: Mode
  cron: string
  runAt: string
  branch: string
  timezone: string
  enabled: boolean
}

const STATUS_VARIANT: Record<string, 'success' | 'warning' | 'destructive'> = {
  fired: 'success',
  skipped_freeze: 'warning',
  skipped_missed: 'warning',
  failed: 'destructive',
}

const HOUR_MS = 3_600_000

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

// datetime-local wants "YYYY-MM-DDTHH:mm" in the viewer's local time.
function toLocalInput(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// A suggestion only: the server owns the real minimum lead time and
// horizon and answers with its own message when a pick is out of range.
function defaultRunAt(now: number): string {
  const d = new Date(now + HOUR_MS)
  d.setMinutes(0, 0, 0)
  if (d.getTime() < now + HOUR_MS) d.setTime(d.getTime() + HOUR_MS)
  return toLocalInput(d)
}

// Next occurrence of hour:00 strictly after now, plus extra whole days.
function nextAtHour(now: number, hour: number, extraDays = 0): string {
  const d = new Date(now)
  d.setHours(hour, 0, 0, 0)
  if (d.getTime() <= now) d.setDate(d.getDate() + 1)
  d.setDate(d.getDate() + extraDays)
  return toLocalInput(d)
}

function tomorrowAt(now: number, hour: number): string {
  const d = new Date(now)
  d.setDate(d.getDate() + 1)
  d.setHours(hour, 0, 0, 0)
  return toLocalInput(d)
}

// ScheduledDeployCard edits an app's scheduled redeploy (GET/PUT
// /api/v1/apps/{name}/schedule): the latest commit on a branch,
// redeployed on a cron expression or once at a set time. Requires a
// connected git source; the API rejects a save otherwise with a clear
// message this card surfaces (inline for one-time, toast for recurring).
export function ScheduledDeployCard({ appName }: { appName: string }) {
  const { t } = useTranslation('scheduledDeploy')
  const schedule = useAppSchedule(appName)
  const save = useSetAppSchedule(appName)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [error, setError] = useState('')
  const [initialRunAt] = useState(() => defaultRunAt(Date.now()))
  const runAtRef = useRef<HTMLInputElement>(null)

  if (schedule.isLoading) return <CardSkeleton titleWidth="w-44" rows={3} />

  const current = schedule.data
  const completed = Boolean(current?.run_at && current.completed_at)
  const pendingOnce = Boolean(current?.run_at && !current.completed_at)
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const values: Draft = draft ?? {
    mode: current?.run_at ? 'once' : 'cron',
    cron: current?.cron || '0 3 * * *',
    runAt:
      pendingOnce && current?.run_at
        ? toLocalInput(new Date(current.run_at))
        : initialRunAt,
    branch: current?.branch ?? 'main',
    timezone: current?.timezone ?? 'UTC',
    enabled: current?.enabled ?? true,
  }

  const update = (patch: Partial<Draft>) => {
    setDraft({ ...values, ...patch })
    setError('')
  }

  const fail = (message: string) => {
    toast.add({
      title: t('saveFailed'),
      description: message,
      type: 'error',
    })
  }

  const onSave = () => {
    const done = {
      onSuccess: () => {
        setDraft(null)
        toast.add({ title: t('saved'), type: 'success' })
      },
    }
    if (values.mode === 'once') {
      const when = new Date(values.runAt)
      if (!values.runAt || Number.isNaN(when.getTime())) {
        setError(t('runAtInvalid'))
        return
      }
      if (!values.branch.trim()) {
        setError(t('branchRequired'))
        return
      }
      save.mutate(
        {
          run_at: when.toISOString(),
          branch: values.branch,
          enabled: values.enabled,
        },
        { ...done, onError: (e) => setError(e.message) },
      )
      return
    }
    if (!values.cron.trim() || !values.branch.trim()) {
      toast.add({ title: t('cronRequired'), type: 'error' })
      return
    }
    save.mutate(
      {
        cron: values.cron,
        branch: values.branch,
        timezone: values.timezone,
        enabled: values.enabled,
      },
      { ...done, onError: (e) => fail(e.message) },
    )
  }

  const scheduleAgain = () => {
    setDraft({ ...values, mode: 'once', runAt: defaultRunAt(Date.now()) })
    runAtRef.current?.focus()
  }

  let nextRun: string
  if (completed) nextRun = t('nextRunDone')
  else if (current) {
    nextRun = t('nextRun', {
      when: formatDate(current.run_at ?? current.next_run_at, t('nextRunNone')),
    })
  } else nextRun = t('nextRun', { when: t('nextRunUnsaved') })

  const errorId = 'schedule-run-at-error'
  const chips: Array<[string, () => string]> = [
    [t('chipHour'), () => toLocalInput(new Date(Date.now() + HOUR_MS))],
    [t('chipTonight'), () => nextAtHour(Date.now(), 3)],
    [t('chipTomorrow'), () => tomorrowAt(Date.now(), 9)],
  ]

  return (
    <Card className="shadow-card ring-1 ring-foreground/10 rounded-xl">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CountdownGlassIcon className="size-4.5 text-muted-foreground" />
          {t('title')}
          <InfoTip label={t('cronTipLabel')}>{t('cronTip')}</InfoTip>
          {current ? (
            <Badge variant={current.enabled ? 'success' : 'muted'}>
              {current.enabled ? t('enabled') : t('disabled')}
            </Badge>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{t('intro')}</p>

        <Tabs
          value={values.mode}
          onValueChange={(v) => update({ mode: v as Mode })}
        >
          <TabsList aria-label={t('modeLabel')}>
            <TabsTrigger value="cron">{t('recurring')}</TabsTrigger>
            <TabsTrigger value="once">{t('oneTime')}</TabsTrigger>
          </TabsList>

          <TabsContent value="cron">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_10rem]">
              <Field>
                <FieldLabel htmlFor="schedule-cron">{t('cron')}</FieldLabel>
                <Input
                  id="schedule-cron"
                  className="font-mono"
                  value={values.cron}
                  onChange={(e) => update({ cron: e.target.value })}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="schedule-branch">{t('branch')}</FieldLabel>
                <Input
                  id="schedule-branch"
                  value={values.branch}
                  onChange={(e) => update({ branch: e.target.value })}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="schedule-tz">{t('timezone')}</FieldLabel>
                <Input
                  id="schedule-tz"
                  value={values.timezone}
                  placeholder="UTC"
                  onChange={(e) => update({ timezone: e.target.value })}
                />
              </Field>
            </div>
          </TabsContent>

          <TabsContent value="once" className="space-y-2">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr]">
              <Field>
                <FieldLabel htmlFor="schedule-run-at">{t('runAt')}</FieldLabel>
                <div className="flex items-center gap-2">
                  <Input
                    id="schedule-run-at"
                    ref={runAtRef}
                    type="datetime-local"
                    value={values.runAt}
                    aria-invalid={error ? true : undefined}
                    aria-describedby={error ? errorId : undefined}
                    onChange={(e) => update({ runAt: e.target.value })}
                  />
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {t('runAtZone', { zone })}
                  </span>
                </div>
              </Field>
              <Field>
                <FieldLabel htmlFor="schedule-branch-once">
                  {t('branch')}
                </FieldLabel>
                <Input
                  id="schedule-branch-once"
                  value={values.branch}
                  onChange={(e) => update({ branch: e.target.value })}
                />
              </Field>
            </div>
            <div
              role="group"
              aria-label={t('chipsLabel')}
              className="flex flex-wrap gap-2"
            >
              {chips.map(([label, pick]) => (
                <Button
                  key={label}
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => update({ runAt: pick() })}
                >
                  {label}
                </Button>
              ))}
            </div>
            {error ? (
              <p id={errorId} role="alert" className="text-sm text-destructive">
                {error}
              </p>
            ) : null}
          </TabsContent>
        </Tabs>

        <div className="flex items-center justify-between gap-4">
          <p className="text-sm font-medium text-foreground">{t('enabled')}</p>
          <Switch
            checked={values.enabled}
            onCheckedChange={(v) => update({ enabled: v })}
            aria-label={t('enabledSwitch')}
          />
        </div>

        {pendingOnce && current?.run_at ? (
          <p className="text-sm text-foreground">
            {t('runsOnce', {
              when: formatDate(current.run_at, current.run_at),
              relative: formatRelativeTime(current.run_at),
            })}
          </p>
        ) : null}
        {completed ? (
          <div className="flex flex-wrap items-center gap-2 text-sm text-foreground">
            <span>
              {t('completed', {
                when: formatDate(current?.completed_at, ''),
              })}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={scheduleAgain}
            >
              {t('scheduleAgain')}
            </Button>
          </div>
        ) : null}

        <p className="text-sm text-muted-foreground">{nextRun}</p>

        <div className="flex flex-wrap gap-2">
          <Button size="sm" onClick={onSave} disabled={save.isPending}>
            {save.isPending ? t('saving') : t('save')}
          </Button>
          {draft !== null ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setDraft(null)
                setError('')
              }}
            >
              {t('discard')}
            </Button>
          ) : null}
        </div>

        {current ? <ScheduleHistoryList appName={appName} /> : null}
      </CardContent>
    </Card>
  )
}

function ScheduleHistoryList({ appName }: { appName: string }) {
  const { t } = useTranslation('scheduledDeploy')
  const history = useAppScheduleHistory(appName, true)
  const entries = history.data ?? []
  const tip = (status: string) =>
    status === 'skipped_freeze' || status === 'skipped_missed'
      ? t(`statusTip.${status}`)
      : undefined

  if (history.isLoading) return <ListSkeleton rows={1} />
  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground italic">{t('noRuns')}</p>
  }

  return (
    <div className="space-y-1">
      <p className="text-sm font-medium text-foreground">{t('recent')}</p>
      <ul className="space-y-1">
        {entries.map((e) => (
          <li
            key={e.id}
            className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
          >
            <Badge
              variant={STATUS_VARIANT[e.status] ?? 'muted'}
              title={tip(e.status)}
            >
              {t(`status.${e.status}`, { defaultValue: e.status })}
            </Badge>
            <span>{formatDate(e.fired_at, e.fired_at)}</span>
            {tip(e.status) ? <span>&middot; {tip(e.status)}</span> : null}
            {e.reason ? <span>&middot; {e.reason}</span> : null}
          </li>
        ))}
      </ul>
    </div>
  )
}
