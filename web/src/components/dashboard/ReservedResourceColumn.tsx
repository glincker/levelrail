import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Progress } from '@/components/ui/progress'
import { TONE } from '../kit/tone'
import type { Tone } from '../kit/tone'
import type { ResourceTotals, Verdict } from '../../lib/reservedUsage'

const VERDICT_TONE: Record<Verdict, Tone> = {
  overCommitted: 'danger',
  idle: 'warning',
  headroom: 'success',
  noLimits: 'neutral',
  unknownCapacity: 'neutral',
}

const BAR_TONE: Record<Tone, string> = {
  neutral: '[&_[data-slot=progress-indicator]]:bg-tone-neutral-solid',
  success: '[&_[data-slot=progress-indicator]]:bg-tone-success-solid',
  warning: '[&_[data-slot=progress-indicator]]:bg-tone-warning-solid',
  danger: '[&_[data-slot=progress-indicator]]:bg-tone-danger-solid',
  info: '[&_[data-slot=progress-indicator]]:bg-tone-info-solid',
  accent: '[&_[data-slot=progress-indicator]]:bg-tone-accent-solid',
}

function pct(part: number, whole: number): number {
  return whole > 0 ? Math.min(100, (part / whole) * 100) : 0
}

export function ReservedResourceColumn({
  label,
  totals,
  format,
  capacityHint,
  noLimitKey,
}: {
  label: string
  totals: ResourceTotals
  format: (n: number) => string
  capacityHint?: string
  noLimitKey: 'cpu' | 'memory'
}) {
  const { t } = useTranslation('dashboard')
  const tone = VERDICT_TONE[totals.verdict]
  const scale = totals.capacity ?? Math.max(totals.reserved, totals.used ?? 0)

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{label}</h3>
        <span
          className={cn(
            'rounded-full px-2 py-0.5 text-xs font-medium',
            TONE[tone].soft,
            TONE[tone].text,
          )}
        >
          {t(`fleet.reserved.verdict.${totals.verdict}`, {
            amount: format(totals.overBy),
          })}
        </span>
      </div>

      <dl className="grid grid-cols-3 gap-2 text-sm">
        <div>
          <dt className="text-xs text-muted-foreground">
            {t('fleet.reserved.reserved')}
          </dt>
          <dd className="font-semibold tabular-nums">
            {format(totals.reserved)}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">
            {t('fleet.reserved.used')}
          </dt>
          <dd className="font-semibold tabular-nums">
            {totals.used === undefined ? (
              <span className="font-normal text-muted-foreground">
                {t('fleet.reserved.unknown')}
              </span>
            ) : (
              format(totals.used)
            )}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">
            {t('fleet.reserved.capacity')}
          </dt>
          <dd className="font-semibold tabular-nums">
            {totals.capacity === undefined ? (
              <span className="font-normal text-muted-foreground">
                {t('fleet.reserved.unknown')}
              </span>
            ) : (
              format(totals.capacity)
            )}
          </dd>
        </div>
      </dl>

      <div className="space-y-1.5">
        <Progress
          value={pct(totals.reserved, scale)}
          aria-label={t('fleet.reserved.reservedBar', { resource: label })}
          className={cn(
            '[&_[data-slot=progress-indicator]]:opacity-50',
            BAR_TONE[tone],
          )}
        />
        <Progress
          value={pct(totals.used ?? 0, scale)}
          aria-label={t('fleet.reserved.usedBar', { resource: label })}
          className={BAR_TONE[tone]}
        />
      </div>

      {capacityHint ? (
        <p className="text-xs text-muted-foreground">{capacityHint}</p>
      ) : null}
      {totals.unsetCount > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t(`fleet.reserved.noLimit.${noLimitKey}`, {
            count: totals.unsetCount,
          })}
        </p>
      ) : null}
    </div>
  )
}
