import type { ReactNode } from 'react'
import { ArrowDownIcon, ArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { AnimatedNumber } from './AnimatedNumber'
import { InfoTip } from './InfoTip'
import { SkeletonTile } from './Skeleton'
import { Sparkline } from './Sparkline'
import { deltaTone, type MetricDelta } from './metricDelta'
import { TONE, type Tone } from './tone'

export interface MetricTileProps {
  label: string
  value: string | number
  unit?: string
  delta?: MetricDelta
  series?: number[]
  tone?: Tone
  icon?: ReactNode
  info?: ReactNode
  loading?: boolean
  onClick?: () => void
}

function DeltaChip({ delta }: { delta: MetricDelta }) {
  const tone = deltaTone(delta)
  const t = TONE[tone]
  const Arrow = delta.direction === 'up' ? ArrowUpIcon : ArrowDownIcon
  return (
    <span
      data-testid="metric-delta"
      data-tone={tone}
      className={cn(
        'inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[11px] font-medium tabular-nums',
        t.soft,
        t.text,
      )}
    >
      <Arrow className="size-3" weight="bold" aria-hidden="true" />
      {Math.abs(delta.value)}%
      <span className="sr-only">
        {delta.direction === 'up' ? ' up' : ' down'},{' '}
        {tone === 'success' ? 'good' : tone === 'danger' ? 'bad' : 'no change'}
      </span>
    </span>
  )
}

export function MetricTile({
  label,
  value,
  unit,
  delta,
  series,
  tone = 'neutral',
  icon,
  info,
  loading,
  onClick,
}: MetricTileProps) {
  if (loading) return <SkeletonTile />

  const body = (
    <>
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon && <span className="[&_svg]:size-4">{icon}</span>}
        <span className="truncate">{label}</span>
      </div>
      <div className="flex items-baseline gap-1.5">
        <span className="text-2xl font-semibold tracking-tight">
          {typeof value === 'number' ? <AnimatedNumber value={value} /> : value}
        </span>
        {unit && <span className="text-sm text-muted-foreground">{unit}</span>}
        {delta && (
          <span className="ml-auto self-center">
            <DeltaChip delta={delta} />
          </span>
        )}
      </div>
      {series && (
        <Sparkline
          values={series}
          tone={tone}
          width="fill"
          height={32}
          fill
          showLast
          ariaLabel={`${label} trend`}
        />
      )}
    </>
  )

  const card =
    'relative flex flex-col gap-2 rounded-xl border border-border bg-card p-4 text-left shadow-raised'

  return (
    <div className="relative" data-testid="metric-tile">
      {onClick ? (
        <button
          type="button"
          onClick={onClick}
          className={cn(
            card,
            'w-full cursor-pointer outline-none transition-colors duration-150 hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/60',
          )}
        >
          {body}
        </button>
      ) : (
        <div className={card}>{body}</div>
      )}
      {info && (
        <span className="absolute top-3 right-3">
          <InfoTip label={`About ${label}`}>{info}</InfoTip>
        </span>
      )}
    </div>
  )
}
