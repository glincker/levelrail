import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
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
import { InfoTip } from '@/components/kit'
import { useDeploySafety, useSetDeploySafety } from '../queries/deployGate'
import type {
  DeploySafety,
  DeploySafetyOverrides,
  WatchMode,
} from '../types/deployGate'
import {
  WATCH_FIELDS,
  defaultText,
  toDraft,
  toOverrides,
} from '../lib/deploySafetyForm'

const INHERIT = 'default'
type ModeChoice = WatchMode | typeof INHERIT
const MODES: WatchMode[] = ['off', 'notify', 'auto']

function isMode(v: string | null): v is WatchMode {
  return v === 'off' || v === 'notify' || v === 'auto'
}

/** PostDeployWatchCard sets what happens when a new release misbehaves after it takes traffic. */
export function PostDeployWatchCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploySafety')
  const { data, isPending } = useDeploySafety(appName)

  if (isPending) return <CardSkeleton titleWidth="w-48" rows={3} />

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RotateCcwGlassIcon className="size-5 text-muted-foreground" />
          {t('autoRollback.title')}
          <InfoTip label={t('autoRollback.infoTipLabel')}>
            {t('autoRollback.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {data ? (
          <WatchForm
            key={JSON.stringify(data.overrides)}
            appName={appName}
            safety={data}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}

function WatchForm({
  appName,
  safety,
}: {
  appName: string
  safety: DeploySafety
}) {
  const { t } = useTranslation('deploySafety')
  const save = useSetDeploySafety(appName)
  const [draft, setDraft] = useState(() => toDraft(safety.overrides))
  const [mode, setMode] = useState<ModeChoice>(
    safety.overrides.watch_mode ?? INHERIT,
  )
  const [soft, setSoft] = useState(safety.overrides.watch_soft_rollback)

  function submit(next: DeploySafetyOverrides) {
    save.mutate(next, {
      onSuccess: () =>
        toast.add({ title: t('autoRollback.toast.saved'), type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('autoRollback.toast.error'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  const modeLabel = (m: WatchMode) => t(`autoRollback.mode.${m}`)

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit(
          toOverrides(draft, safety.overrides, WATCH_FIELDS, {
            watch_mode: mode === INHERIT ? null : mode,
            watch_soft_rollback: soft,
            auto_rollback: null,
          }),
        )
      }}
    >
      <Field>
        <FieldLabel htmlFor="auto-rollback-mode">
          {t('autoRollback.mode.label')}
        </FieldLabel>
        <Select
          value={mode}
          onValueChange={(v) => {
            if (v === INHERIT || isMode(v)) setMode(v)
          }}
        >
          <SelectTrigger id="auto-rollback-mode" className="w-full sm:w-72">
            <SelectValue>
              {mode === INHERIT
                ? t('autoRollback.mode.default', {
                    value: modeLabel(safety.defaults.watch_mode),
                  })
                : modeLabel(mode)}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={INHERIT}>
              {t('autoRollback.mode.default', {
                value: modeLabel(safety.defaults.watch_mode),
              })}
            </SelectItem>
            {MODES.map((m) => (
              <SelectItem key={m} value={m}>
                {modeLabel(m)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <p className="text-sm text-muted-foreground">
        {t('autoRollback.inheritHint')}
      </p>
      <div className="grid gap-4 sm:grid-cols-2">
        {WATCH_FIELDS.map(({ key, numeric }) => (
          <Field key={key}>
            <FieldLabel htmlFor={`auto-rollback-${key}`}>
              {t(`settings.field.${key}.label`)}
            </FieldLabel>
            <Input
              id={`auto-rollback-${key}`}
              inputMode={numeric ? 'decimal' : 'text'}
              value={draft[key]}
              placeholder={defaultText(key, safety.defaults)}
              onChange={(e) => setDraft({ ...draft, [key]: e.target.value })}
            />
            <FieldDescription className="text-xs">
              {t(`settings.field.${key}.hint`)}
            </FieldDescription>
          </Field>
        ))}
      </div>
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-foreground">
            {t('autoRollback.soft.label')}
          </p>
          <p className="text-sm text-muted-foreground">
            {soft === null
              ? t('autoRollback.soft.inherited', {
                  value: safety.defaults.watch_soft_rollback
                    ? t('settings.on')
                    : t('settings.off'),
                })
              : t('autoRollback.soft.hint')}
          </p>
        </div>
        <Switch
          checked={soft ?? safety.defaults.watch_soft_rollback}
          onCheckedChange={setSoft}
          aria-label={t('autoRollback.soft.label')}
        />
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={save.isPending}>
          {t('autoRollback.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={save.isPending}
          onClick={() =>
            submit({
              ...safety.overrides,
              watch_mode: null,
              watch_window: null,
              watch_max_restarts: null,
              watch_error_rate: null,
              watch_min_requests: null,
              watch_latency_ratio: null,
              watch_soft_rollback: null,
              auto_rollback: null,
            })
          }
        >
          {t('autoRollback.reset')}
        </Button>
      </div>
    </form>
  )
}
