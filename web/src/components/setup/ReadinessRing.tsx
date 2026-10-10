import { cn } from '@/lib/utils'
import type { CategoryVerdict } from '../../lib/setupReadiness'

const RADIUS = 45
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

const STROKE: Record<CategoryVerdict, string> = {
  ready: 'stroke-tone-success-solid',
  attention: 'stroke-tone-warning-solid',
  blocked: 'stroke-tone-danger-solid',
}

/** ReadinessRing draws the readiness score as a ring with the number in the middle; score null renders an empty ring. */
export function ReadinessRing({
  score,
  verdict,
  label,
  className,
}: {
  score: number | null
  verdict: CategoryVerdict
  label: string
  className?: string
}) {
  const filled = score === null ? 0 : (score / 100) * CIRCUMFERENCE
  return (
    <div
      role="img"
      aria-label={label}
      className={cn('relative size-28 shrink-0', className)}
    >
      <svg
        viewBox="0 0 100 100"
        className="size-full -rotate-90"
        aria-hidden="true"
      >
        <circle
          cx="50"
          cy="50"
          r={RADIUS}
          fill="none"
          strokeWidth="7"
          className="stroke-muted"
        />
        <circle
          cx="50"
          cy="50"
          r={RADIUS}
          fill="none"
          strokeWidth="7"
          strokeLinecap="round"
          strokeDasharray={`${filled} ${CIRCUMFERENCE}`}
          className={STROKE[verdict]}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-3xl font-light tabular-nums tracking-tight text-foreground">
          {score === null ? '-' : score}
        </span>
        {score === null ? null : (
          <span className="text-xs uppercase tracking-wider text-muted-foreground">
            %
          </span>
        )}
      </div>
    </div>
  )
}
