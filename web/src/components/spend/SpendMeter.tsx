import { useTranslation } from 'react-i18next'
import { Progress, ProgressValue } from '@/components/ui/progress'

/** SpendMeter is one horizontal row: progress toward the cap, then where the month is heading. */
export function SpendMeter({
  label,
  detail,
  pct,
  projectedPct,
  projected,
}: {
  label: string
  detail: string
  pct: number
  projectedPct?: number
  projected?: string
}) {
  const { t } = useTranslation('budget')
  const over = projectedPct !== undefined && projectedPct >= 100
  return (
    <div className="grid items-center gap-x-4 gap-y-1 sm:grid-cols-[minmax(0,1fr)_auto]">
      <div className="min-w-0 space-y-1">
        <div className="flex items-baseline justify-between gap-2 text-xs">
          <span className="font-medium">{label}</span>
          <span className="truncate text-muted-foreground">{detail}</span>
        </div>
        <Progress value={Math.min(Math.max(pct, 0), 100)}>
          <ProgressValue>{() => `${Math.round(pct)}%`}</ProgressValue>
        </Progress>
      </div>
      {projected ? (
        <p
          className={
            over
              ? 'text-xs font-medium text-destructive'
              : 'text-xs text-muted-foreground'
          }
        >
          {t('forecast.projected', { value: projected })}
          {over ? ` ${t('forecast.overLimit')}` : ''}
        </p>
      ) : null}
    </div>
  )
}
