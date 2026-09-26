import { useMemo } from 'react'
import { Controller, type Control, type FieldErrors } from 'react-hook-form'
import { InfoTip } from '@/components/kit'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import {
  sloConfigFromForm,
  useSloPreview,
  type SloPreview,
} from '../queries/sloPreview'
import type { SloConfig, SloObjective } from '../types/alerts'

export interface SloFormShape {
  sloObjective: SloObjective
  sloTarget: string | number
  sloLatencyMs: string | number
}

const PREVIEW_DEBOUNCE_MS = 400

const BURN_RATE_TIP =
  'Burn rate is how many times faster than sustainable your error budget is being spent: 1x uses exactly the budget over 30 days, 14.4x empties it in about two days.'

function windowLabel(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400}d`
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  return `${Math.round(seconds / 60)}m`
}

function formatBurn(v: number): string {
  return `${v < 10 ? v.toFixed(2) : v.toFixed(1)}x`
}

function BudgetMeter({ preview }: { preview: SloPreview }) {
  const remaining = preview.budget_remaining * 100
  const shown = Math.max(0, Math.min(100, remaining))
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs">
        <span className="flex items-center gap-1 font-medium text-foreground">
          Error budget remaining
          <InfoTip label="About the error budget">
            The share of allowed failures you have not used yet in the 30 day
            window. At a 99.9% target, 0.1% of requests may fail; when the
            budget hits zero the SLO is broken.
          </InfoTip>
        </span>
        <span
          className={
            remaining < 0
              ? 'font-mono text-destructive'
              : 'font-mono text-foreground'
          }
        >
          {remaining.toFixed(1)}%
        </span>
      </div>
      <Progress value={shown} aria-label="Error budget remaining" />
    </div>
  )
}

// Live "error budget remaining" and per-tier burn rates for the SLO being
// edited, from the same query an slo_burn rule evaluates.
export function SloPreviewPanel({
  app,
  config,
}: {
  app: string
  config: SloConfig
}) {
  const debounced = useDebouncedValue(config, PREVIEW_DEBOUNCE_MS)
  const { data, isLoading, isError } = useSloPreview(app, debounced)

  if (isLoading) {
    return <p className="text-xs text-muted-foreground">Loading preview...</p>
  }
  if (isError || data === null || data === undefined) {
    return (
      <p className="text-xs text-muted-foreground">
        Preview unavailable until the target is valid and request metrics are
        being collected.
      </p>
    )
  }
  if (!data.has_traffic) {
    return (
      <p className="text-xs text-muted-foreground">
        No request traffic recorded for this app yet, so nothing has burned. The
        rule starts evaluating as soon as requests arrive.
      </p>
    )
  }
  return (
    <div
      className="space-y-3 rounded-lg border border-border bg-muted/30 p-3"
      data-testid="slo-preview"
    >
      <BudgetMeter preview={data} />
      <div>
        <p className="mb-1 flex items-center gap-1 text-xs font-medium text-foreground">
          Current burn rates
          <InfoTip label="About burn rate">{BURN_RATE_TIP}</InfoTip>
        </p>
        <ul className="space-y-1 text-xs">
          {data.tiers.map((t) => (
            <li
              key={t.name}
              className="flex items-center justify-between gap-2 text-muted-foreground"
            >
              <span>
                {t.page ? 'Page' : 'Ticket'} at {t.factor}x over{' '}
                {windowLabel(t.long_seconds)} and {windowLabel(t.short_seconds)}
              </span>
              <span
                className={
                  t.firing
                    ? 'font-mono font-semibold text-destructive'
                    : 'font-mono text-foreground'
                }
              >
                {formatBurn(t.long_burn)} / {formatBurn(t.short_burn)}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}

interface SloRuleFieldsProps {
  idPrefix: string
  app: string
  control: Control<SloFormShape>
  errors: FieldErrors<SloFormShape>
  objective: SloObjective
  target: string | number
  latencyMs: string | number
}

export function SloRuleFields({
  idPrefix,
  app,
  control,
  errors,
  objective,
  target,
  latencyMs,
}: SloRuleFieldsProps) {
  const config = useMemo(
    () => sloConfigFromForm(objective, target, latencyMs),
    [objective, target, latencyMs],
  )
  return (
    <>
      <p className="text-sm text-muted-foreground">
        Alerts when the app spends its 30 day error budget too fast, judged over
        two windows at once so a brief blip does not page anyone and a real
        outage does.
      </p>
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-slo-objective`}>Objective</FieldLabel>
        <Controller
          control={control}
          name="sloObjective"
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger
                id={`${idPrefix}-slo-objective`}
                className="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="availability">
                  Availability (requests without a 5xx)
                </SelectItem>
                <SelectItem value="latency">
                  Latency (requests under a time limit)
                </SelectItem>
              </SelectContent>
            </Select>
          )}
        />
      </Field>
      <Field>
        <div className="flex items-center gap-1">
          <FieldLabel htmlFor={`${idPrefix}-slo-target`}>
            Target (% of good requests)
          </FieldLabel>
          <InfoTip label="About burn rate">{BURN_RATE_TIP}</InfoTip>
        </div>
        <Controller
          control={control}
          name="sloTarget"
          render={({ field }) => (
            <Input
              id={`${idPrefix}-slo-target`}
              type="number"
              step="0.01"
              min="50"
              max="99.999"
              value={field.value}
              onChange={field.onChange}
              onBlur={field.onBlur}
            />
          )}
        />
        <FieldDescription>
          For example 99.9 allows one failed request in a thousand.
        </FieldDescription>
        <FieldError errors={[errors.sloTarget]} />
      </Field>
      {objective === 'latency' ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-slo-latency`}>
            Latency limit (ms)
          </FieldLabel>
          <Controller
            control={control}
            name="sloLatencyMs"
            render={({ field }) => (
              <Input
                id={`${idPrefix}-slo-latency`}
                type="number"
                step="1"
                min="5"
                value={field.value}
                onChange={field.onChange}
                onBlur={field.onBlur}
              />
            )}
          />
          <FieldDescription>
            Rounded down to the nearest histogram bound (5, 10, 25, 50, 100,
            250, 500 ms, 1, 2.5, 5, 10 s).
          </FieldDescription>
          <FieldError errors={[errors.sloLatencyMs]} />
        </Field>
      ) : null}
      <SloPreviewPanel app={app} config={config} />
    </>
  )
}
