import { cn } from '@/lib/utils'
import type { Tone } from '../kit/tone'

const FILL: Record<Tone, string> = {
  neutral: 'fill-tone-neutral-solid',
  success: 'fill-tone-success-solid',
  warning: 'fill-tone-warning-solid',
  danger: 'fill-tone-danger-solid',
  info: 'fill-tone-info-solid',
  accent: 'fill-tone-accent-solid',
}

export interface UsageSegment {
  key: string
  /** Share of the bar, 0 to 100. */
  percent: number
  tone: Tone
  faded?: boolean
  dimmed?: boolean
  /** Pointer shortcut only: the legend holds the keyboard and screen reader path. */
  onSelect?: () => void
}

/** SVG geometry keeps widths out of inline styles; the fill grows with a transform-only animation. */
export function UsageBar({
  segments,
  label,
  className,
}: {
  segments: UsageSegment[]
  label: string
  className?: string
}) {
  const starts = segments.map((_, i) =>
    segments.slice(0, i).reduce((sum, x) => sum + x.percent, 0),
  )
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 100 1"
      preserveAspectRatio="none"
      className={cn(
        'block w-full overflow-hidden rounded-full bg-muted',
        className,
      )}
    >
      <g className="cleanup-grow">
        {segments.map((s, i) => {
          const x = starts[i]
          if (s.percent <= 0) return null
          return (
            <rect
              key={s.key}
              x={x}
              y={0}
              width={Math.max(s.percent - 0.3, 0.2)}
              height={1}
              aria-hidden="true"
              onClick={s.onSelect}
              className={cn(
                FILL[s.tone],
                s.faded && 'opacity-60',
                s.dimmed && 'opacity-25',
                s.onSelect && 'cursor-pointer',
              )}
            />
          )
        })}
      </g>
    </svg>
  )
}
