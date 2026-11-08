import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { toast } from '@/components/ui/toast'
import {
  policyQueryOptions,
  useSetPolicy,
  type PolicyMode,
  type UpgradePolicy,
} from '../../queries/upgradeSchedule'
import {
  updateSettingsQueryOptions,
  useSetUpdateSettings,
  type UpdateChannel,
  type UpdateSettings,
} from '../../queries/updates'

const FALLBACK_CHANNELS: UpdateChannel[] = ['stable']

function AutoUpdateForm({
  policy,
  settings,
}: {
  policy: UpgradePolicy
  settings: UpdateSettings
}) {
  const { t } = useTranslation('updates')
  const savePolicy = useSetPolicy()
  const saveSettings = useSetUpdateSettings()
  const [mode, setMode] = useState<PolicyMode>(
    policy.mode === 'off' ? 'patch' : policy.mode,
  )
  const [on, setOn] = useState(policy.mode !== 'off')
  const [cron, setCron] = useState(policy.window_cron)
  const [minutes, setMinutes] = useState(String(policy.window_duration_minutes))
  const [pauseDays, setPauseDays] = useState('')
  const [health, setHealth] = useState(
    policy.health_window_seconds ? String(policy.health_window_seconds) : '',
  )
  const [channel, setChannel] = useState<UpdateChannel>(settings.channel)
  const channels = settings.offered_channels?.length
    ? settings.offered_channels
    : FALLBACK_CHANNELS

  function save(e: React.FormEvent) {
    e.preventDefault()
    const days = Number.parseInt(pauseDays, 10)
    const healthSeconds = Number.parseInt(health, 10)
    savePolicy.mutate(
      {
        mode: on ? mode : 'off',
        window_cron: cron.trim(),
        window_duration_minutes:
          Number.parseInt(minutes, 10) || policy.window_duration_minutes,
        timezone: policy.timezone,
        min_release_age_days: policy.min_release_age_days,
        notify_channel_ids: policy.notify_channel_ids,
        paused: policy.paused,
        ...(Number.isNaN(days) ? {} : { pause_days: days }),
        ...(Number.isNaN(healthSeconds)
          ? {}
          : { health_window_seconds: healthSeconds }),
      },
      {
        onSuccess: () => {
          setPauseDays('')
          toast.add({ title: t('autoUpdate.saved'), type: 'success' })
        },
        onError: (err) =>
          toast.add({
            title: t('autoUpdate.error', { message: err.message }),
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

  const paused = policy.paused_until
    ? t('autoUpdate.statePaused', {
        date: new Date(policy.paused_until).toLocaleDateString(),
      })
    : null

  return (
    <form className="space-y-3" onSubmit={save}>
      <div className="flex flex-wrap items-center gap-3">
        <Switch
          checked={on}
          onCheckedChange={setOn}
          aria-label={t('autoUpdate.state')}
        />
        <span className="text-sm font-medium text-foreground">
          {paused ?? (on ? t('autoUpdate.stateOn') : t('autoUpdate.stateOff'))}
        </span>
        {policy.default_applied ? (
          <span className="text-xs text-muted-foreground">
            {t('autoUpdate.defaultApplied')}
          </span>
        ) : null}
      </div>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <Field>
          <FieldLabel htmlFor="au-mode">{t('autoUpdate.modeLabel')}</FieldLabel>
          <Select
            value={mode}
            onValueChange={(v) => {
              if (v === 'patch' || v === 'any') setMode(v)
            }}
          >
            <SelectTrigger id="au-mode" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="patch">
                {t('autoUpdate.mode.patch')}
              </SelectItem>
              <SelectItem value="any">{t('autoUpdate.mode.any')}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="au-cron">{t('autoUpdate.window')}</FieldLabel>
          <Input
            id="au-cron"
            className="font-mono"
            value={cron}
            onChange={(e) => setCron(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="au-min">
            {t('autoUpdate.windowMinutes')}
          </FieldLabel>
          <Input
            id="au-min"
            type="number"
            min={1}
            value={minutes}
            onChange={(e) => setMinutes(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="au-pause">{t('autoUpdate.pause')}</FieldLabel>
          <Input
            id="au-pause"
            type="number"
            min={0}
            placeholder={t('autoUpdate.pauseHelp')}
            value={pauseDays}
            onChange={(e) => setPauseDays(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="au-health">{t('autoUpdate.health')}</FieldLabel>
          <Input
            id="au-health"
            type="number"
            min={1}
            value={health}
            onChange={(e) => setHealth(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="au-channel">
            {t('autoUpdate.channel')}
          </FieldLabel>
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
                  {c}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-prose text-xs text-muted-foreground">
          {t('autoUpdate.safety')}
        </p>
        <Button
          type="submit"
          size="sm"
          disabled={savePolicy.isPending || saveSettings.isPending}
        >
          {t('autoUpdate.save')}
        </Button>
      </div>
    </form>
  )
}

/** AutoUpdateCard edits automatic update state, window, pause, health window and channel. */
export function AutoUpdateCard() {
  const { t } = useTranslation('updates')
  const policy = useQuery(policyQueryOptions())
  const settings = useQuery(updateSettingsQueryOptions())
  if (!policy.data || !settings.data) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckGlassIcon className="size-5 text-muted-foreground" />
          {t('autoUpdate.title')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <AutoUpdateForm
          key={`${policy.data.updated_at ?? ''}${policy.data.mode}`}
          policy={policy.data}
          settings={settings.data}
        />
      </CardContent>
    </Card>
  )
}
