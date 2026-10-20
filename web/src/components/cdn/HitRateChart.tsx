import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { CacheBucket } from '../../queries/cdnCache'
import { formatCount } from './format'

const VIEW_W = 120
const VIEW_H = 40
const BAR_FILL = 0.72

function Swatch({ className }: Readonly<{ className: string }>) {
  return (
    <span
      className={cn('inline-block size-2 rounded-sm', className)}
      aria-hidden="true"
    />
  )
}

/**
 * HitRateChart stacks cache hits (served by the ingress) and misses (fetched
 * from the app) per minute. Bars are SVG rects sized by attribute, so the
 * chart scales with its container and needs no inline styles.
 */
export function HitRateChart({
  series,
  className,
}: Readonly<{ series: CacheBucket[]; className?: string }>) {
  const { t } = useTranslation('cdn')
  const hits = series.reduce((s, b) => s + b.hits, 0)
  const misses = series.reduce((s, b) => s + b.misses, 0)
  const max = Math.max(1, ...series.map((b) => b.hits + b.misses))
  const slot = series.length > 0 ? VIEW_W / series.length : VIEW_W
  const barW = slot * BAR_FILL

  if (hits + misses === 0) {
    return (
      <div
        className={cn(
          'flex h-24 items-center justify-center rounded-lg bg-muted/40 px-4 text-center text-xs text-muted-foreground',
          className,
        )}
      >
        {t('chart.empty')}
      </div>
    )
  }

  return (
    <figure className={cn('space-y-2', className)}>
      <svg
        viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={t('chart.aria', {
          hits: formatCount(hits),
          misses: formatCount(misses),
          minutes: series.length,
        })}
        className="block h-24 w-full overflow-visible"
      >
        <line
          x1={0}
          x2={VIEW_W}
          y1={VIEW_H}
          y2={VIEW_H}
          className="stroke-border"
          strokeWidth={0.5}
          vectorEffect="non-scaling-stroke"
        />
        {series.map((b, i) => {
          const hitH = (b.hits / max) * VIEW_H
          const missH = (b.misses / max) * VIEW_H
          const x = i * slot + (slot - barW) / 2
          return (
            <g key={b.minute}>
              <rect
                x={x}
                y={VIEW_H - hitH}
                width={barW}
                height={hitH}
                className="fill-tone-success-solid"
              />
              <rect
                x={x}
                y={VIEW_H - hitH - missH}
                width={barW}
                height={missH}
                className="fill-tone-info-solid"
              />
            </g>
          )
        })}
      </svg>
      <figcaption className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <span>{t('chart.span', { minutes: series.length })}</span>
        <span className="flex items-center gap-3">
          <span className="inline-flex items-center gap-1.5">
            <Swatch className="bg-tone-success-solid" />
            {t('chart.hits')}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <Swatch className="bg-tone-info-solid" />
            {t('chart.misses')}
          </span>
        </span>
      </figcaption>
    </figure>
  )
}
