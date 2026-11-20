import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon, ClockGlassIcon } from '@thesvg/ui-react'
import { ApiErrorAlert } from '@/components/ApiErrorAlert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast, toastAction } from '@/components/ui/toast'
import {
  buildWindowCron,
  DEFAULT_MINUTES,
  DURATION_CHOICES,
  formatClock,
  isValidTimezone,
  parseClock,
  parseWindowCron,
  windowEnd,
  type WindowDays,
} from '@/lib/autoUpdateWindow'
import {
  updateSettingsQueryOptions,
  useSetUpdateSettings,
  type UpdateChannel,
  type UpdateSettings,
} from '@/queries/updates'
import {
  policyQueryOptions,
  useSetPolicy,
  type PolicyMode,
  type UpgradePolicy,
  type UpgradePolicyInput,
} from '@/queries/upgradeSchedule'
import { cn } from '@/lib/utils'
import { AutoUpdateStatus } from './AutoUpdateStatus'

const LEVELS = ['patch', 'any'] as const
const DAY_CHOICES: WindowDays[] = ['daily', 'weekdays', 'weekends']

function inputFrom(
  policy: UpgradePolicy,
  over: Partial<UpgradePolicyInput>,
): UpgradePolicyInput {
  return {
    mode: policy.mode,
    window_cron: policy.window_cron,
    window_duration_minutes: policy.window_duration_minutes,
    timezone: policy.timezone,
    min_release_age_days: policy.min_release_age_days,
    notify_channel_ids: policy.notify_channel_ids,
    paused: policy.paused,
    ...(policy.health_window_seconds
      ? { health_window_seconds: policy.health_window_seconds }
      : {}),
    ...over,
  }
}

function ScheduleForm({
  policy,
  settings,
}: {
  policy: UpgradePolicy
  settings: UpdateSettings
}) {
  const { t } = useTranslation('updates')
  const savePolicy = useSetPolicy()
  const saveSettings = useSetUpdateSettings()
  const parts = parseWindowCron(policy.window_cron)
  const [level, setLevel] = useState<PolicyMode>(
    policy.mode === 'any' ? 'any' : 'patch',
  )
  const [start, setStart] = useState(formatClock(parts.hour, parts.minute))
  const [minutes, setMinutes] = useState(
    policy.window_duration_minutes || DEFAULT_MINUTES,
  )
  const [days, setDays] = useState<WindowDays>(parts.days)
  const [zone, setZone] = useState(policy.timezone || policy.server_timezone)
  const [channel, setChannel] = useState<UpdateChannel>(settings.channel)
  const channels = settings.offered_channels?.length
    ? settings.offered_channels
    : [settings.channel]
  const radioName = useId()
  const clock = parseClock(start)
  const zoneOk = isValidTimezone(zone)
  const valid = clock !== null && zoneOk
  const durations = DURATION_CHOICES.includes(minutes)
    ? DURATION_CHOICES
    : [...DURATION_CHOICES, minutes].sort((a, b) => a - b)

  function save(e: React.FormEvent) {
    e.preventDefault()
    if (!clock) return
    savePolicy.mutate(
      inputFrom(policy, {
        mode: level,
        window_cron: buildWindowCron({ ...clock, days }, policy.window_cron),
        window_duration_minutes: minutes,
        timezone: zone.trim(),
      }),
      {
        onSuccess: () =>
          toast.add({ title: t('quick.scheduleSaved'), type: 'success' }),
        onError: (err) =>
          toast.add({
            title: t('quick.error', { message: err.message }),
            type: 'error',
          }),
      },
    )
    if (channel !== settings.channel) {
      saveSettings.mutate({
        channel,
        auto_update_enabled: settings.auto_update_enabled,
      })
    }
  }

  return (
    <form className="space-y-3 border-t border-border pt-3" onSubmit={save}>
      <fieldset className="space-y-1.5">
        <legend className="text-sm font-medium">
          {t('quick.level.label')}
        </legend>
        <div className="inline-flex rounded-lg border border-border p-0.5">
          {LEVELS.map((l) => (
            <label key={l} className="relative">
              <input
                type="radio"
                name={radioName}
                value={l}
                checked={level === l}
                onChange={() => setLevel(l)}
                className="peer sr-only"
              />
              <span className="block cursor-pointer rounded-md px-3 py-1 text-sm text-muted-foreground peer-checked:bg-primary peer-checked:text-primary-foreground peer-focus-visible:ring-2 peer-focus-visible:ring-ring">
                {t(`quick.level.${l}`)}
              </span>
            </label>
          ))}
        </div>
      </fieldset>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Field>
          <FieldLabel htmlFor="au-start">{t('quick.start')}</FieldLabel>
          <Input
            id="au-start"
            type="time"
            value={start}
            aria-invalid={clock === null}
            onChange={(e) => setStart(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="au-length">{t('quick.length')}</FieldLabel>
          <Select
            value={String(minutes)}
            onValueChange={(v) => {
              const n = Number(v)
              if (!Number.isNaN(n)) setMinutes(n)
            }}
          >
            <SelectTrigger id="au-length" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {durations.map((d) => (
                <SelectItem key={d} value={String(d)}>
                  {t('quick.hours', { hours: d / 60 })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="au-days">{t('quick.daysLabel')}</FieldLabel>
          <Select
            value={days}
            onValueChange={(v) => {
              const hit = [...DAY_CHOICES, 'custom' as const].find(
                (d) => d === v,
              )
              if (hit) setDays(hit)
            }}
          >
            <SelectTrigger id="au-days" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DAY_CHOICES.map((d) => (
                <SelectItem key={d} value={d}>
                  {t(`quick.dayOption.${d}`)}
                </SelectItem>
              ))}
              {parts.days === 'custom' ? (
                <SelectItem value="custom">
                  {t('quick.dayOption.custom')}
                </SelectItem>
              ) : null}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="au-zone">{t('quick.zone')}</FieldLabel>
          <Input
            id="au-zone"
            value={zone}
            aria-invalid={!zoneOk}
            aria-describedby="au-zone-help"
            onChange={(e) => setZone(e.target.value)}
          />
        </Field>
      </div>
      <p
        id="au-zone-help"
        role={zoneOk ? undefined : 'alert'}
        className={cn(
          'text-xs',
          zoneOk ? 'text-muted-foreground' : 'text-destructive',
        )}
      >
        {zoneOk
          ? t('quick.zoneHelp', { zone: policy.server_timezone })
          : t('quick.zoneInvalid')}
      </p>
      <div className="flex flex-wrap items-end justify-between gap-3">
        {channels.length > 1 ? (
          <Field className="w-full sm:w-48">
            <FieldLabel htmlFor="au-channel">{t('quick.channel')}</FieldLabel>
            <Select
              value={channel}
              onValueChange={(v) => {
                const hit = channels.find((c) => c === v)
                if (hit) setChannel(hit)
              }}
            >
              <SelectTrigger id="au-channel" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {channels.map((c) => (
                  <SelectItem key={c} value={c}>
                    {t(`channels.${c}.label`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : (
          <span />
        )}
        <Button
          type="submit"
          size="sm"
          disabled={!valid || savePolicy.isPending}
        >
          {t('quick.save')}
        </Button>
      </div>
    </form>
  )
}

function summaryParts(policy: UpgradePolicy) {
  const p = parseWindowCron(policy.window_cron)
  const minutes = policy.window_duration_minutes || DEFAULT_MINUTES
  return {
    days: p.days,
    start: formatClock(p.hour, p.minute),
    end: windowEnd(p.hour, p.minute, minutes),
  }
}

function AutoUpdatesBody({
  policy,
  settings,
}: {
  policy: UpgradePolicy
  settings: UpdateSettings
}) {
  const { t } = useTranslation('updates')
  const savePolicy = useSetPolicy()
  const saveSettings = useSetUpdateSettings()
  const [optimistic, setOptimistic] = useState<boolean | null>(null)
  const [open, setOpen] = useState(false)
  const formId = useId()
  const on = optimistic ?? policy.mode !== 'off'
  const win = summaryParts(policy)

  function apply(next: boolean, withUndo: boolean) {
    setOptimistic(next)
    const mode: PolicyMode = next
      ? policy.mode === 'any'
        ? 'any'
        : 'patch'
      : 'off'
    savePolicy.mutate(inputFrom(policy, { mode }), {
      onSuccess: () => {
        setOptimistic(null)
        toast.add({
          title: t(next ? 'quick.savedOn' : 'quick.savedOff'),
          type: 'success',
          ...(withUndo
            ? {
                actionProps: toastAction(t('quick.undo'), () =>
                  apply(!next, false),
                ),
              }
            : {}),
        })
      },
      onError: (err) => {
        setOptimistic(null)
        toast.add({
          title: t('quick.error', { message: err.message }),
          type: 'error',
        })
      },
    })
    // Background checks must be on for a window to ever find a release.
    if (next && !settings.auto_update_enabled) {
      saveSettings.mutate({
        channel: settings.channel,
        auto_update_enabled: true,
      })
    }
  }

  let sentence: string
  if (policy.paused_until) {
    sentence = t('quick.paused', {
      date: new Date(policy.paused_until).toLocaleDateString(),
    })
  } else if (!on) {
    sentence = t('quick.off')
  } else {
    sentence = t(policy.mode === 'any' ? 'quick.onAny' : 'quick.onPatch', {
      start: win.start,
      end: win.end,
      days: t(`quick.days.${win.days}`),
    })
  }
  const next = on && policy.next_window ? policy.next_window.start : null

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex items-start gap-3">
          <Switch
            id="update-auto-switch"
            checked={on}
            disabled={savePolicy.isPending}
            onCheckedChange={(v) => apply(v, true)}
            className="mt-0.5"
          />
          <div className="min-w-0 flex-1 space-y-1">
            <label htmlFor="update-auto-switch" className="text-sm font-medium">
              {t('quick.title')}
            </label>
            <p
              role="status"
              className="max-w-prose text-sm text-muted-foreground"
            >
              {sentence}
            </p>
            {next ? (
              <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <ClockGlassIcon aria-hidden="true" className="size-4" />
                {t('quick.next', {
                  time: new Date(next).toLocaleString([], {
                    weekday: 'short',
                    hour: '2-digit',
                    minute: '2-digit',
                  }),
                })}
              </p>
            ) : null}
          </div>
        </div>
        {on ? (
          <div className="space-y-0.5 text-xs text-muted-foreground">
            <p>
              {t('quick.installSteps', {
                start: win.start,
                end: win.end,
                zone: policy.timezone || 'UTC',
              })}
            </p>
            {policy.next_windows && policy.next_windows.length > 0 ? (
              <p>
                {t('quick.nextWindows', {
                  list: policy.next_windows
                    .map(
                      (w) =>
                        `${new Date(w.start).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit' })} / ${new Date(w.start).toISOString().slice(11, 16)} UTC`,
                    )
                    .join(', '),
                })}
              </p>
            ) : null}
          </div>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {t(settings.auto_update_enabled ? 'quick.checkOn' : 'quick.checkOff')}
        </p>
        {policy.status ? <AutoUpdateStatus status={policy.status} /> : null}
        {on ? (
          <div>
            <button
              type="button"
              aria-expanded={open}
              aria-controls={formId}
              className="flex w-full items-center gap-2 rounded-md px-1 py-1 text-left text-sm outline-none hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => setOpen((v) => !v)}
            >
              <ChevronRightGlassIcon
                aria-hidden="true"
                className={cn(
                  'size-4 text-muted-foreground transition-transform motion-reduce:transition-none',
                  open && 'rotate-90',
                )}
              />
              <span className="font-medium">{t('quick.scheduleToggle')}</span>
              <span className="truncate text-muted-foreground">
                {t('quick.scheduleSummary', {
                  start: win.start,
                  end: win.end,
                  days: t(`quick.days.${win.days}`),
                })}
              </span>
            </button>
            <div id={formId} hidden={!open} className="pt-2">
              {open ? (
                <ScheduleForm
                  key={`${policy.updated_at ?? ''}${policy.mode}`}
                  policy={policy}
                  settings={settings}
                />
              ) : null}
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

/** AutoUpdatesCard is the page's master switch plus a collapsed night-window schedule. */
export function AutoUpdatesCard() {
  const { t } = useTranslation('updates')
  const policy = useQuery(policyQueryOptions())
  const settings = useQuery(updateSettingsQueryOptions())
  if (policy.isError) {
    return (
      <Card>
        <CardContent className="space-y-3">
          <ApiErrorAlert error={policy.error} />
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void policy.refetch()}
          >
            {t('page.retry')}
          </Button>
        </CardContent>
      </Card>
    )
  }
  if (!policy.data || !settings.data) {
    return (
      <Card aria-hidden="true">
        <CardContent>
          <div className="h-10 animate-pulse rounded-md bg-muted motion-reduce:animate-none" />
        </CardContent>
      </Card>
    )
  }
  return <AutoUpdatesBody policy={policy.data} settings={settings.data} />
}
