import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type {
  Autoscale,
  AutoscaleMetric,
  AutoscalePolicyFields,
} from '../../queries/autoscale'

const METRICS: AutoscaleMetric[] = [
  'cpu_percent',
  'memory_percent',
  'requests_per_replica',
]
const METRIC_KEYS = {
  cpu_percent: ['form.metricCpu', 'form.metricCpuHelp'],
  memory_percent: ['form.metricMemory', 'form.metricMemoryHelp'],
  requests_per_replica: ['form.metricRequests', 'form.metricRequestsHelp'],
} as const

interface Draft {
  min: string
  max: string
  metric: AutoscaleMetric
  target: string
  upTolerance: string
  downTolerance: string
  upCooldown: string
  downCooldown: string
  stabilization: string
  stepUp: string
  stepDown: string
}

const emptyAdvanced = {
  upTolerance: '',
  downTolerance: '',
  upCooldown: '',
  downCooldown: '',
  stabilization: '',
  stepUp: '',
  stepDown: '',
}

function minutes(secs: number): string {
  return String(Math.round((secs / 60) * 100) / 100)
}

function toDraft(policy: Autoscale | null): Draft {
  if (!policy) {
    return {
      min: '1',
      max: '5',
      metric: 'cpu_percent',
      target: '60',
      ...emptyAdvanced,
    }
  }
  return {
    min: String(policy.min_replicas),
    max: String(policy.max_replicas),
    metric: policy.metric,
    target: String(policy.target),
    upTolerance: String(policy.scale_up_tolerance_pct),
    downTolerance: String(policy.scale_down_tolerance_pct),
    upCooldown: minutes(policy.scale_up_cooldown_secs),
    downCooldown: minutes(policy.scale_down_cooldown_secs),
    stabilization: minutes(policy.stabilization_secs),
    stepUp: String(policy.max_step_up),
    stepDown: String(policy.max_step_down),
  }
}

function whole(raw: string, min: number, max: number): number | null {
  if (raw.trim() === '') return 0
  const n = Number(raw)
  return Number.isInteger(n) && n >= min && n <= max ? n : null
}

function toSeconds(raw: string): number | null {
  if (raw.trim() === '') return 0
  const n = Number(raw)
  if (!Number.isFinite(n) || n < 1 / 60 || n > 1440) return null
  return Math.round(n * 60)
}

type Build =
  | { fields: AutoscalePolicyFields }
  | { error: 'replicas' | 'target' | 'advanced' }

function build(d: Draft): Build {
  const min = whole(d.min, 1, 1000)
  const max = whole(d.max, 1, 1000)
  if (!min || !max || max < min) return { error: 'replicas' }
  const target = Number(d.target)
  const limit = d.metric === 'requests_per_replica' ? Infinity : 100
  if (!Number.isFinite(target) || target <= 0 || target > limit) {
    return { error: 'target' }
  }
  const upTol = whole(d.upTolerance, 1, 100)
  const downTol = whole(d.downTolerance, 1, 99)
  const upCool = toSeconds(d.upCooldown)
  const downCool = toSeconds(d.downCooldown)
  const stab = toSeconds(d.stabilization)
  const stepUp = whole(d.stepUp, 1, 100)
  const stepDown = whole(d.stepDown, 1, 100)
  if (
    upTol === null ||
    downTol === null ||
    upCool === null ||
    downCool === null ||
    stab === null ||
    stepUp === null ||
    stepDown === null
  ) {
    return { error: 'advanced' }
  }
  return {
    fields: {
      min_replicas: min,
      max_replicas: max,
      metric: d.metric,
      target,
      scale_up_tolerance_pct: upTol,
      scale_down_tolerance_pct: downTol,
      scale_up_cooldown_secs: upCool,
      scale_down_cooldown_secs: downCool,
      stabilization_secs: stab,
      max_step_up: stepUp,
      max_step_down: stepDown,
    },
  }
}

function Field({
  id,
  label,
  help,
  value,
  onChange,
  placeholder,
}: {
  id: string
  label: string
  help?: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        inputMode="decimal"
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        aria-describedby={help ? `${id}-help` : undefined}
      />
      {help ? (
        <p id={`${id}-help`} className="text-xs text-muted-foreground">
          {help}
        </p>
      ) : null}
    </div>
  )
}

/** AutoscaleForm edits an app's policy; it validates locally and leaves the request to the caller. */
export function AutoscaleForm({
  policy,
  pending,
  serverError,
  onSave,
  onCancel,
}: {
  policy: Autoscale | null
  pending: boolean
  serverError: string | null
  onSave: (fields: AutoscalePolicyFields) => void
  onCancel: () => void
}) {
  const { t } = useTranslation('autoscale')
  const [draft, setDraft] = useState(() => toDraft(policy))
  const [error, setError] = useState<string | null>(null)
  const set = (patch: Partial<Draft>) => setDraft((d) => ({ ...d, ...patch }))

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const built = build(draft)
    if ('error' in built) {
      setError(t(`form.error.${built.error}`))
      return
    }
    setError(null)
    onSave(built.fields)
  }

  const shown = error ?? serverError
  const unit =
    draft.metric === 'requests_per_replica' ? t('form.perSecond') : '%'
  return (
    <form onSubmit={submit} className="space-y-4" noValidate>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          id="autoscale-min"
          label={t('form.min')}
          help={t('form.minHelp')}
          value={draft.min}
          onChange={(min) => set({ min })}
        />
        <Field
          id="autoscale-max"
          label={t('form.max')}
          help={t('form.maxHelp')}
          value={draft.max}
          onChange={(max) => set({ max })}
        />
      </div>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t('form.metric')}</legend>
        <div className="grid gap-2 sm:grid-cols-3">
          {METRICS.map((m) => (
            <label
              key={m}
              className="flex cursor-pointer items-start gap-2 rounded-lg border border-border p-2.5 text-sm has-[:checked]:border-ring has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"
            >
              <input
                type="radio"
                name="autoscale-metric"
                value={m}
                checked={draft.metric === m}
                onChange={() => set({ metric: m })}
                className="mt-0.5"
              />
              <span className="min-w-0 space-y-0.5">
                <span className="block font-medium">
                  {t(METRIC_KEYS[m][0])}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(METRIC_KEYS[m][1])}
                </span>
              </span>
            </label>
          ))}
        </div>
      </fieldset>
      <div className="max-w-xs">
        <Field
          id="autoscale-target"
          label={t('form.target', { unit })}
          help={t('form.targetHelp')}
          value={draft.target}
          onChange={(target) => set({ target })}
        />
      </div>
      <details className="rounded-lg border border-border p-3">
        <summary className="cursor-pointer text-sm font-medium">
          {t('form.advanced')}
        </summary>
        <p className="mt-2 text-xs text-muted-foreground">
          {t('form.advancedHelp')}
        </p>
        <div className="mt-3 grid gap-4 sm:grid-cols-2">
          <Field
            id="autoscale-up-tolerance"
            label={t('form.upTolerance')}
            placeholder="10"
            value={draft.upTolerance}
            onChange={(upTolerance) => set({ upTolerance })}
          />
          <Field
            id="autoscale-down-tolerance"
            label={t('form.downTolerance')}
            placeholder="30"
            value={draft.downTolerance}
            onChange={(downTolerance) => set({ downTolerance })}
          />
          <Field
            id="autoscale-up-cooldown"
            label={t('form.upCooldown')}
            placeholder="1"
            value={draft.upCooldown}
            onChange={(upCooldown) => set({ upCooldown })}
          />
          <Field
            id="autoscale-down-cooldown"
            label={t('form.downCooldown')}
            placeholder="10"
            value={draft.downCooldown}
            onChange={(downCooldown) => set({ downCooldown })}
          />
          <Field
            id="autoscale-stabilization"
            label={t('form.stabilization')}
            placeholder="5"
            value={draft.stabilization}
            onChange={(stabilization) => set({ stabilization })}
          />
          <div className="hidden sm:block" aria-hidden="true" />
          <Field
            id="autoscale-step-up"
            label={t('form.stepUp')}
            placeholder="4"
            value={draft.stepUp}
            onChange={(stepUp) => set({ stepUp })}
          />
          <Field
            id="autoscale-step-down"
            label={t('form.stepDown')}
            placeholder="1"
            value={draft.stepDown}
            onChange={(stepDown) => set({ stepDown })}
          />
        </div>
      </details>
      {shown ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{shown}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={pending}>
          {pending ? t('form.saving') : t('form.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={pending}
          onClick={onCancel}
        >
          {t('form.cancel')}
        </Button>
      </div>
    </form>
  )
}
