import { Display } from '@/components/ui/typography'
import { formatDeltaPercent } from '@/lib/deltaPercent'
import type { ReactNode } from 'react'
import { ArrowDownGlassIcon, ArrowUpGlassIcon } from '@thesvg/ui-react'
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
  // Small line under the value, for qualifiers such as an instance-wide marker.
  note?: ReactNode
  loading?: boolean
  onClick?: () => void
  // Shown when the tile has nothing to measure yet; ignored with onClick.
  cta?: { label: string; onAction: () => void }
}

function DeltaChip({ delta }: { delta: MetricDelta }) {
  const tone = deltaTone(delta)
  const t = TONE[tone]
  const Arrow = delta.direction === 'up' ? ArrowUpGlassIcon : ArrowDownGlassIcon
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
      <Arrow className="size-3.5" aria-hidden="true" />
      {formatDeltaPercent(delta.value)}
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
  note,
  loading,
  onClick,
  cta,
}: MetricTileProps) {
  if (loading) return <SkeletonTile />

  const body = (
    <>
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon && <span className="[&_svg]:size-5">{icon}</span>}
        <span className="truncate">{label}</span>
      </div>
      <div className="flex items-baseline gap-1.5">
        <Display as="span" size="page" className="tabular-nums">
          {typeof value === 'number' ? <AnimatedNumber value={value} /> : value}
        </Display>
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
      {note}
    </>
  )

  const card =
    'relative flex h-full flex-col gap-2 rounded-[var(--glinui-radius-md)] border border-border bg-card p-4 text-left shadow-raised transition-shadow duration-150'

  return (
    <div className="relative h-full" data-testid="metric-tile">
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
        <div className={card}>
          {body}
          {cta ? (
            <button
              type="button"
              onClick={cta.onAction}
              className="mt-auto self-start text-xs font-medium text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
            >
              {cta.label}
            </button>
          ) : null}
        </div>
      )}
      {info && (
        <span className="absolute top-3 right-3">
          <InfoTip label={`About ${label}`}>{info}</InfoTip>
        </span>
      )}
    </div>
  )
}
