import { InfoTip } from '@/components/kit'
import { MAX_WEIGHT, type LbFormState } from '../../lib/loadBalancer'
import { weightShares } from './detection'
import { LbTextField, type LbPatch } from './LbField'
import type { LbFormErrors } from '../../lib/loadBalancer'

export function DistributionBar({ weights }: { weights: number[] }) {
  const shares = weightShares(weights)
  return (
    <div
      role="img"
      aria-label={`Traffic split: ${shares.map((s, i) => `replica ${i} ${s}%`).join(', ')}`}
      className="flex h-8 w-full overflow-hidden rounded-lg border"
    >
      {weights.map((w, i) => (
        <div
          key={i}
          data-segment={i}
          style={{ flexGrow: Math.max(w, 0.0001), flexBasis: 0 }}
          className={
            i % 2 === 0
              ? 'flex items-center justify-center bg-primary/25 text-xs font-medium tabular-nums'
              : 'flex items-center justify-center bg-primary/10 text-xs font-medium tabular-nums'
          }
        >
          {shares[i] ? `R${i} ${shares[i]}%` : ''}
        </div>
      ))}
    </div>
  )
}

function SlowStartRamp({ seconds }: { seconds: string }) {
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <svg viewBox="0 0 48 16" className="h-4 w-12" aria-hidden="true">
        <polyline
          points="0,15 6,15 40,2 48,2"
          fill="none"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="stroke-primary"
        />
      </svg>
      New replicas ramp up over {seconds}
    </div>
  )
}

export function WeightSliders({
  form,
  errors,
  onChange,
}: {
  form: LbFormState
  errors: LbFormErrors
  onChange: LbPatch
}) {
  const shares = weightShares(form.weights)
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-1 text-sm font-medium">
        Traffic split
        <InfoTip label="About weights">
          Weights follow replica index. A replica with weight 3 gets three times
          the traffic of one with weight 1.
        </InfoTip>
      </div>
      <DistributionBar weights={form.weights} />
      <ul className="space-y-2">
        {form.weights.map((weight, index) => (
          <li key={index} className="flex items-center gap-3">
            <span className="w-16 text-sm text-muted-foreground">
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
            <span className="w-7 text-right text-sm tabular-nums">
              {weight}
            </span>
            <span className="w-10 text-right text-xs text-muted-foreground tabular-nums">
              {shares[index] ?? 0}%
            </span>
          </li>
        ))}
      </ul>
      <div className="max-w-56">
        <LbTextField
          id="lb-slow-start"
          label="Slow start"
          info="A new replica ramps from zero up to its weight over this time instead of taking its full share at once."
          value={form.slowStart}
          error={errors.slowStart}
          placeholder="30s"
          onChange={(slowStart) => onChange({ slowStart })}
        />
      </div>
      {form.slowStart.trim() ? (
        <SlowStartRamp seconds={form.slowStart.trim()} />
      ) : null}
    </div>
  )
}
