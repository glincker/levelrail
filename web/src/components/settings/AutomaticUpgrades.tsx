import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'
import { Checkbox } from '../ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '../ui/field'
import { Input } from '../ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'
import { Switch } from '../ui/switch'
import { toast } from '../ui/toast'
import { updateSettingsQueryOptions } from '../../queries/updates'
import { useNotificationChannelsOptional } from '../../queries/notificationChannels'
import {
  policyQueryOptions,
  useSetPolicy,
  type PolicyMode,
  type UpgradePolicy,
} from '../../queries/upgradeSchedule'
import { formatInZone } from '../../lib/formatInZone'

const PRESETS: { id: 'nightly' | 'weekly' | 'weekdays'; cron: string }[] = [
  { id: 'nightly', cron: '0 3 * * *' },
  { id: 'weekly', cron: '0 3 * * 0' },
  { id: 'weekdays', cron: '0 2 * * 1-5' },
]

function presetFor(cron: string): string {
  return PRESETS.find((p) => p.cron === cron)?.id ?? 'custom'
}

function PolicyForm({ policy }: { policy: UpgradePolicy }) {
  const { t } = useTranslation('updates')
  const save = useSetPolicy()
  const channels = useNotificationChannelsOptional()
  const [mode, setMode] = useState<PolicyMode>(policy.mode)
  const [cron, setCron] = useState(policy.window_cron || '0 3 * * *')
  const [duration, setDuration] = useState(
    String(policy.window_duration_minutes || 120),
  )
  const [timezone, setTimezone] = useState(
    policy.timezone || policy.server_timezone,
  )
  const [minAge, setMinAge] = useState(String(policy.min_release_age_days))
  const [paused, setPaused] = useState(policy.paused)
  const [notify, setNotify] = useState<string[]>(policy.notify_channel_ids)

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save.mutate(
      {
        mode,
        window_cron: cron.trim(),
        window_duration_minutes: Number.parseInt(duration, 10) || 120,
        timezone: timezone.trim(),
        min_release_age_days: Number.parseInt(minAge, 10) || 0,
        notify_channel_ids: notify,
        paused,
      },
      {
        onSuccess: () => toast.add({ title: t('auto.saved'), type: 'success' }),
      },
    )
  }

  return (
    <form className="space-y-4" onSubmit={submit}>
      <p className="text-sm text-muted-foreground" role="status">
        {t(`auto.notice.${policy.effective}`)}
      </p>
      <Field>
        <FieldLabel htmlFor="auto-mode">{t('auto.modeLabel')}</FieldLabel>
        <Select
          value={mode}
          onValueChange={(v) => {
            if (v === 'off' || v === 'patch' || v === 'any') setMode(v)
          }}
        >
          <SelectTrigger id="auto-mode" className="w-full sm:w-96">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="off">{t('auto.mode.off')}</SelectItem>
            <SelectItem value="patch">{t('auto.mode.patch')}</SelectItem>
            <SelectItem value="any">{t('auto.mode.any')}</SelectItem>
          </SelectContent>
        </Select>
        <FieldDescription>{t('auto.modeHelp')}</FieldDescription>
      </Field>
      {mode !== 'off' ? (
        <>
          <Field>
            <FieldLabel htmlFor="auto-preset">
              {t('auto.windowLabel')}
            </FieldLabel>
            <Select
              value={presetFor(cron)}
              onValueChange={(v) => {
                const hit = PRESETS.find((p) => p.id === v)
                if (hit) setCron(hit.cron)
              }}
            >
              <SelectTrigger id="auto-preset" className="w-full sm:w-96">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PRESETS.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {t(`auto.preset.${p.id}`)}
                  </SelectItem>
                ))}
                <SelectItem value="custom">
                  {t('auto.preset.custom')}
                </SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field>
              <FieldLabel htmlFor="auto-cron">{t('auto.cronLabel')}</FieldLabel>
              <Input
                id="auto-cron"
                value={cron}
                onChange={(e) => setCron(e.target.value)}
                className="font-mono"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="auto-duration">
                {t('auto.durationLabel')}
              </FieldLabel>
              <Input
                id="auto-duration"
                type="number"
                min={10}
                max={1440}
                value={duration}
                onChange={(e) => setDuration(e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="auto-tz">
                {t('auto.timezoneLabel')}
              </FieldLabel>
              <Input
                id="auto-tz"
                value={timezone}
                onChange={(e) => setTimezone(e.target.value)}
              />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="auto-age">{t('auto.minAgeLabel')}</FieldLabel>
            <Input
              id="auto-age"
              type="number"
              min={0}
              max={90}
              value={minAge}
              onChange={(e) => setMinAge(e.target.value)}
              className="w-32"
            />
            <FieldDescription>{t('auto.minAgeHelp')}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel>{t('auto.notifyLabel')}</FieldLabel>
            {(channels.data ?? []).length === 0 ? (
              <FieldDescription>{t('auto.notifyNone')}</FieldDescription>
            ) : (
              <ul className="space-y-1.5">
                {(channels.data ?? []).map((c) => (
                  <li key={c.id} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      id={`auto-notify-${c.id}`}
                      checked={notify.includes(c.id)}
                      onCheckedChange={(v) =>
                        setNotify((prev) =>
                          v === true
                            ? [...prev, c.id]
                            : prev.filter((id) => id !== c.id),
                        )
                      }
                    />
                    <label htmlFor={`auto-notify-${c.id}`}>{c.name}</label>
                  </li>
                ))}
              </ul>
            )}
            <FieldDescription>{t('auto.notifyHelp')}</FieldDescription>
          </Field>
          <div className="flex items-center gap-2 text-sm">
            <Switch
              id="auto-paused"
              checked={paused}
              onCheckedChange={setPaused}
            />
            <label htmlFor="auto-paused">{t('auto.pauseLabel')}</label>
          </div>
        </>
      ) : null}
      {policy.next_window && mode !== 'off' ? (
        <p className="text-xs text-muted-foreground">
          {t('auto.nextWindow', {
            time: formatInZone(policy.next_window.start, timezone),
            timezone,
          })}
        </p>
      ) : null}
      {policy.decision ? (
        <p className="text-xs text-muted-foreground">
          {t('auto.decision', {
            version: policy.decision_tag ?? '',
            decision: policy.decision,
          })}
        </p>
      ) : null}
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <Button type="submit" disabled={save.isPending}>
        {t('auto.save')}
      </Button>
    </form>
  )
}

export function AutomaticUpgrades() {
  const { t } = useTranslation('updates')
  const policy = useQuery(policyQueryOptions())
  const settings = useQuery(updateSettingsQueryOptions())
  if (settings.data?.channel === 'edge') {
    return (
      <p className="text-sm text-muted-foreground" role="status">
        {t('auto.edgeUnavailable')}
      </p>
    )
  }
  if (policy.isPending) {
    return <p className="text-sm text-muted-foreground">{t('auto.loading')}</p>
  }
  if (policy.isError) {
    return (
      <p className="text-sm text-muted-foreground">{t('auto.unavailable')}</p>
    )
  }
  return <PolicyForm policy={policy.data} />
}
