import { useState } from 'react'
import { ClockCountdownIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { InfoTip } from './kit'
import { formatDate } from '../lib/format'
import {
  useAppSchedule,
  useAppScheduleHistory,
  useSetAppSchedule,
  type AppScheduleInput,
} from '../queries/appSchedule'

const STATUS_LABEL: Record<string, string> = {
  fired: 'Fired',
  skipped_freeze: 'Skipped (freeze window)',
  failed: 'Failed',
}

const STATUS_VARIANT: Record<string, 'success' | 'warning' | 'destructive'> = {
  fired: 'success',
  skipped_freeze: 'warning',
  failed: 'destructive',
}

// ScheduledDeployCard edits an app's recurring redeploy (GET/PUT
// /api/v1/apps/{name}/schedule): the latest commit on a branch,
// redeployed whenever a cron expression fires. Requires a connected git
// source; the API rejects a save otherwise with a clear message this
// card just surfaces via the mutation's own error toast.
export function ScheduledDeployCard({ appName }: { appName: string }) {
  const schedule = useAppSchedule(appName)
  const save = useSetAppSchedule(appName)
  const [draft, setDraft] = useState<AppScheduleInput | null>(null)

  if (schedule.isLoading) return null

  const current = schedule.data
  const values: AppScheduleInput = draft ?? {
    cron: current?.cron ?? '0 3 * * *',
    branch: current?.branch ?? 'main',
    timezone: current?.timezone ?? 'UTC',
    enabled: current?.enabled ?? true,
  }
  const dirty = draft !== null

  const update = (patch: Partial<AppScheduleInput>) => {
    setDraft({ ...values, ...patch })
  }

  const onSave = () => {
    if (!values.cron.trim() || !values.branch.trim()) {
      toast.add({ title: 'Cron and branch are both required.', type: 'error' })
      return
    }
    save.mutate(values, {
      onSuccess: () => {
        setDraft(null)
        toast.add({ title: 'Scheduled deploy saved.', type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not save the schedule.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ClockCountdownIcon className="size-4 text-muted-foreground" />
          Scheduled deploys
          <InfoTip label="About cron syntax">
            Standard 5-field cron: minute hour day-of-month month day-of-week,
            each either a number, a range (1-5), a step (*/15), or * for any
            value. &quot;0 3 * * *&quot; means every day at 03:00. The schedule
            is checked at least once a minute, so nothing finer than minute
            granularity is meaningful.
          </InfoTip>
          {current ? (
            <Badge variant={current.enabled ? 'success' : 'muted'}>
              {current.enabled ? 'Enabled' : 'Disabled'}
            </Badge>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Redeploys the latest commit on a branch whenever the cron expression
          fires. A deploy due during an active deploy freeze window is skipped,
          not forced, and recorded below.
        </p>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_10rem]">
          <Field>
            <FieldLabel htmlFor="schedule-cron">Cron</FieldLabel>
            <Input
              id="schedule-cron"
              className="font-mono"
              value={values.cron}
              onChange={(e) => update({ cron: e.target.value })}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="schedule-branch">Branch</FieldLabel>
            <Input
              id="schedule-branch"
              value={values.branch}
              onChange={(e) => update({ branch: e.target.value })}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="schedule-tz">Timezone</FieldLabel>
            <Input
              id="schedule-tz"
              value={values.timezone ?? 'UTC'}
              placeholder="UTC"
              onChange={(e) => update({ timezone: e.target.value })}
            />
          </Field>
        </div>

        <div className="flex items-center justify-between gap-4">
          <p className="text-sm font-medium text-foreground">Enabled</p>
          <Switch
            checked={values.enabled ?? true}
            onCheckedChange={(v) => update({ enabled: v })}
            aria-label="Schedule enabled"
          />
        </div>

        <p className="text-sm text-muted-foreground">
          Next run:{' '}
          {current
            ? formatDate(current.next_run_at, 'not yet scheduled')
            : 'save a schedule to see its next run'}
        </p>

        <div className="flex flex-wrap gap-2">
          <Button size="sm" onClick={onSave} disabled={save.isPending}>
            {save.isPending ? 'Saving...' : 'Save'}
          </Button>
          {dirty ? (
            <Button variant="ghost" size="sm" onClick={() => setDraft(null)}>
              Discard changes
            </Button>
          ) : null}
        </div>

        {current ? <ScheduleHistoryList appName={appName} /> : null}
      </CardContent>
    </Card>
  )
}

function ScheduleHistoryList({ appName }: { appName: string }) {
  const history = useAppScheduleHistory(appName, true)
  const entries = history.data ?? []

  if (history.isLoading) return null
  if (entries.length === 0) {
    return (
      <p className="text-sm text-muted-foreground italic">
        No scheduled deploys have run yet.
      </p>
    )
  }

  return (
    <div className="space-y-1">
      <p className="text-sm font-medium text-foreground">Recent runs</p>
      <ul className="space-y-1">
        {entries.map((e) => (
          <li
            key={e.id}
            className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
          >
            <Badge variant={STATUS_VARIANT[e.status] ?? 'muted'}>
              {STATUS_LABEL[e.status] ?? e.status}
            </Badge>
            <span>{formatDate(e.fired_at, e.fired_at)}</span>
            {e.reason ? <span>&middot; {e.reason}</span> : null}
          </li>
        ))}
      </ul>
    </div>
  )
}
