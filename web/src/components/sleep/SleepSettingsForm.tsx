import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { IDLE_PRESETS, isValidIdleMinutes } from '../../lib/sleepState'
import type { AppSleep, SleepUpdate } from '../../queries/appSleep'

type Patch = Partial<Omit<SleepUpdate, 'idle_minutes'>> & {
  idle_minutes?: number
}

function Setting({
  title,
  help,
  children,
}: {
  title: string
  help: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-start justify-between gap-4 border-t border-border pt-3">
      <div className="min-w-0 space-y-0.5">
        <p className="text-sm font-medium text-foreground">{title}</p>
        <p className="text-xs text-muted-foreground">{help}</p>
      </div>
      {children}
    </div>
  )
}

function IgnorePaths({
  paths,
  disabled,
  onChange,
}: {
  paths: string[]
  disabled: boolean
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation('deploys')
  const [draft, setDraft] = useState('')
  const add = () => {
    const value = draft.trim()
    if (!value || paths.includes(value)) return
    setDraft('')
    onChange([...paths, value])
  }
  return (
    <div className="space-y-2 border-t border-border pt-3">
      <div className="space-y-0.5">
        <p className="text-sm font-medium text-foreground">
          {t('appSleep.ignore.title')}
        </p>
        <p className="text-xs text-muted-foreground">
          {t('appSleep.ignore.help')}
        </p>
      </div>
      <ul
        className="flex flex-wrap gap-1.5"
        aria-label={t('appSleep.ignore.title')}
      >
        {paths.map((p) => (
          <li
            key={p}
            className="flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 font-mono text-xs"
          >
            {p}
            <button
              type="button"
              disabled={disabled}
              aria-label={t('appSleep.ignore.remove', { path: p })}
              className="rounded-sm text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => onChange(paths.filter((x) => x !== p))}
            >
              <XGlassIcon aria-hidden="true" className="size-4.5" />
            </button>
          </li>
        ))}
      </ul>
      <div className="flex gap-2">
        <Input
          value={draft}
          placeholder="/healthz"
          aria-label={t('appSleep.ignore.addLabel')}
          className="max-w-56 font-mono"
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || !draft.trim()}
          onClick={add}
        >
          <PlusGlassIcon aria-hidden="true" className="size-5" />
          {t('appSleep.ignore.add')}
        </Button>
      </div>
    </div>
  )
}

/** SleepSettingsForm edits the idle time, request holding, port wake and ignored paths of an enabled app. */
export function SleepSettingsForm({
  sleep,
  pending,
  onSave,
}: {
  sleep: AppSleep
  pending: boolean
  onSave: (patch: Patch, success: string) => void
}) {
  const { t } = useTranslation('deploys')
  const { idle_minutes: idle, hold_requests: hold } = sleep.settings
  const [custom, setCustom] = useState(String(idle))
  const customValue = Number(custom)
  const customValid = isValidIdleMinutes(customValue, sleep.idle_limits)
  const portWake = sleep.port_wake
  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <p
          className="text-sm font-medium text-foreground"
          id="sleep-idle-label"
        >
          {t('appSleep.idleTitle')}
        </p>
        <div
          className="flex flex-wrap items-end gap-2"
          role="group"
          aria-labelledby="sleep-idle-label"
        >
          {IDLE_PRESETS.map((m) => (
            <Button
              key={m}
              type="button"
              size="sm"
              variant={idle === m ? 'default' : 'outline'}
              aria-pressed={idle === m}
              disabled={pending}
              onClick={() => {
                setCustom(String(m))
                onSave({ idle_minutes: m }, t('appSleep.toast.updated'))
              }}
            >
              {t('appSleep.presetMinutes', { count: m })}
            </Button>
          ))}
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('appSleep.minutesLabel', {
              min: sleep.idle_limits.min_minutes,
              max: sleep.idle_limits.max_minutes,
            })}
            <Input
              type="number"
              min={sleep.idle_limits.min_minutes}
              max={sleep.idle_limits.max_minutes}
              value={custom}
              aria-invalid={!customValid}
              className="w-28"
              onChange={(e) => setCustom(e.target.value)}
            />
          </label>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={pending || !customValid || customValue === idle}
            onClick={() =>
              onSave({ idle_minutes: customValue }, t('appSleep.toast.updated'))
            }
          >
            {t('appSleep.save')}
          </Button>
        </div>
      </div>
      <Setting title={t('appSleep.holdLabel')} help={t('appSleep.holdHelp')}>
        <Switch
          checked={hold}
          disabled={pending}
          aria-label={t('appSleep.holdAriaLabel')}
          onCheckedChange={(next) =>
            onSave({ hold_requests: next }, t('appSleep.toast.holdUpdated'))
          }
        />
      </Setting>
      {portWake.supported ? (
        <Setting
          title={t('appSleep.portWake.title')}
          help={t('appSleep.portWake.help', {
            ports: portWake.ports.join(', '),
          })}
        >
          <Switch
            checked={sleep.settings.port_wake}
            disabled={pending}
            aria-label={t('appSleep.portWake.aria')}
            onCheckedChange={(next) =>
              onSave({ port_wake: next }, t('appSleep.toast.portWakeUpdated'))
            }
          />
        </Setting>
      ) : null}
      <IgnorePaths
        paths={sleep.settings.ignore_paths}
        disabled={pending}
        onChange={(next) =>
          onSave({ ignore_paths: next }, t('appSleep.toast.ignoreUpdated'))
        }
      />
    </div>
  )
}
