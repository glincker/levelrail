import { useTranslation } from 'react-i18next'
import { InfoTip } from '@/components/kit'
import { MAX_WEIGHT, type LbFormState } from '../../lib/loadBalancer'
import { weightShares } from './detection'
import { LbTextField, type LbPatch } from './LbField'
import type { LbFormErrors } from '../../lib/loadBalancer'

export function DistributionBar({ weights }: { weights: number[] }) {
  const { t } = useTranslation('statePlaceholders')
  const shares = weightShares(weights)
  return (
    <div
      role="img"
      aria-label={t('lb.weights.split', {
        parts: shares
          .map((s, i) => t('lb.weights.part', { index: i, share: s }))
          .join(', '),
      })}
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
  const { t } = useTranslation('statePlaceholders')
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
      {t('lb.weights.ramp', { duration: seconds })}
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
  const { t } = useTranslation('statePlaceholders')
  const shares = weightShares(form.weights)
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-1 text-sm font-medium">
        {t('lb.weights.title')}
        <InfoTip label={t('lb.weights.about')}>
          {t('lb.weights.aboutBody')}
        </InfoTip>
      </div>
      <DistributionBar weights={form.weights} />
      <ul className="space-y-2">
        {form.weights.map((weight, index) => (
          <li key={index} className="flex items-center gap-3">
            <span className="w-16 text-sm text-muted-foreground">
              {t('lb.weights.replica', { index })}
            </span>
            <input
              type="range"
              min={1}
              max={MAX_WEIGHT}
              value={weight}
              aria-label={t('lb.weights.weightFor', { index })}
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
          label={t('lb.weights.slowStart')}
          info={t('lb.weights.slowStartInfo')}
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
