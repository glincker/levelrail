import { cn } from '@/lib/utils'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  ALGORITHM_OPTIONS,
  MAX_WEIGHT,
  type LbFormErrors,
} from '../lib/loadBalancer'
import type { LbFieldsProps } from './LoadBalancerFieldBits'

function totalShare(weights: number[], index: number): string {
  const total = weights.reduce((a, b) => a + b, 0)
  if (total === 0) return '0%'
  return `${Math.round(((weights[index] ?? 0) / total) * 100)}%`
}

function WeightSliders({ form, onChange }: LbFieldsProps) {
  return (
    <Field>
      <FieldLabel>Weight per replica</FieldLabel>
      <FieldDescription>
        Replica 0 is the first container. A replica with weight 3 gets three
        times the traffic of a replica with weight 1.
      </FieldDescription>
      <ul className="space-y-2">
        {form.weights.map((weight, index) => (
          <li key={index} className="flex items-center gap-3">
            <span className="w-20 text-sm text-muted-foreground">
              Replica {index}
            </span>
            <input
              type="range"
              min={1}
              max={MAX_WEIGHT}
              value={weight}
              aria-label={`Weight for replica ${index}`}
              className="h-2 flex-1 cursor-pointer accent-primary"
              onChange={(e) => {
                const next = [...form.weights]
                next[index] = Number(e.target.value)
                onChange({ weights: next })
              }}
            />
            <span className="w-8 text-right text-sm tabular-nums">
              {weight}
            </span>
            <span className="w-12 text-right text-xs text-muted-foreground tabular-nums">
              {totalShare(form.weights, index)}
            </span>
          </li>
        ))}
      </ul>
    </Field>
  )
}

export function LoadBalancerAlgorithmFields({
  form,
  errors,
  onChange,
}: LbFieldsProps) {
  const errorFor = (key: keyof LbFormErrors) =>
    errors[key] ? [{ message: errors[key] }] : []
  return (
    <div className="space-y-4">
      <div
        role="radiogroup"
        aria-label="Balancing algorithm"
        className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3"
      >
        {ALGORITHM_OPTIONS.map((option) => {
          const selected = form.algorithm === option.value
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() => onChange({ algorithm: option.value })}
              className={cn(
                'rounded-lg border p-3 text-left text-sm transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
                selected ? 'border-primary bg-primary/5' : 'hover:bg-muted/50',
              )}
            >
              <span className="block font-medium">{option.label}</span>
              <span className="block text-xs text-muted-foreground">
                {option.description}
              </span>
            </button>
          )
        })}
      </div>

      {form.algorithm === 'cookie' ? (
        <Field>
          <FieldLabel htmlFor="lb-cookie-name">Cookie name</FieldLabel>
          <Input
            id="lb-cookie-name"
            value={form.cookieName}
            placeholder="lb"
            onChange={(e) => onChange({ cookieName: e.target.value })}
          />
          <FieldError errors={errorFor('cookieName')} />
        </Field>
      ) : null}

      {form.algorithm === 'weighted' ? (
        <WeightSliders form={form} errors={errors} onChange={onChange} />
      ) : null}
    </div>
  )
}
