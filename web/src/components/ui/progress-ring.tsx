import { cn } from '@/lib/utils'

// Circumference of 100 so the dash length equals the percentage.
const RADIUS = 15.9155

function clamp(value: number) {
  return Math.min(100, Math.max(0, value))
}

// Determinate ring. Only the dash length animates, and not under reduced motion.
export function ProgressRing({
  value,
  label,
  className,
}: {
  value: number
  label: string
  className?: string
}) {
  const pct = clamp(value)
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(pct)}
      className={cn(
        'relative inline-flex size-10 items-center justify-center',
        className,
      )}
    >
      <svg
        viewBox="0 0 36 36"
        className="size-full -rotate-90"
        aria-hidden="true"
      >
        <circle
          cx="18"
          cy="18"
          r={RADIUS}
          fill="none"
          strokeWidth="3"
          className="stroke-muted"
        />
        <circle
          cx="18"
          cy="18"
          r={RADIUS}
          fill="none"
          strokeWidth="3"
          strokeLinecap="round"
          strokeDasharray={`${pct} 100`}
          className="stroke-state-accent transition-[stroke-dasharray] motion-reduce:transition-none"
        />
      </svg>
      <span className="absolute text-xs font-medium tabular-nums">
        {Math.round(pct)}
      </span>
    </div>
  )
}

// Linear variant built on the native element so no inline width is needed.
export function ProgressBar({
  value,
  label,
  className,
}: {
  value: number
  label: string
  className?: string
}) {
  return (
    <progress
      aria-label={label}
      max={100}
      value={clamp(value)}
      className={cn(
        'h-1.5 w-full appearance-none overflow-hidden rounded-full [&::-moz-progress-bar]:bg-state-accent [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:rounded-full [&::-webkit-progress-value]:bg-state-accent',
        className,
      )}
    />
  )
}
