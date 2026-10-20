import { cn } from '@/lib/utils'

const base = 'relative overflow-hidden rounded-md bg-muted'

export function SkeletonLine({
  width = '100%',
  className,
}: {
  width?: string | number
  className?: string
}) {
  return (
    <div
      data-testid="skeleton-line"
      aria-hidden="true"
      className={cn(base, 'h-3', className)}
      style={{ width }}
    >
      <div className="kit-shimmer absolute inset-0" />
    </div>
  )
}

export function SkeletonTile({ className }: { className?: string }) {
  return (
    <div
      data-testid="skeleton-tile"
      aria-hidden="true"
      className={cn(
        'flex flex-col gap-3 rounded-xl border border-border bg-card p-4',
        className,
      )}
    >
      <SkeletonLine width="40%" />
      <SkeletonLine width="60%" className="h-6" />
      <SkeletonLine width="100%" className="h-8" />
    </div>
  )
}

export function SkeletonList({ rows = 3 }: { rows?: number }) {
  return (
    <div role="status" aria-label="Loading" className="flex flex-col gap-3">
      {Array.from({ length: rows }, (_, i) => (
        <div
          key={i}
          className="flex items-center gap-3"
          data-testid="skeleton-row"
        >
          <div className={cn(base, 'size-8 shrink-0 rounded-full')}>
            <div className="kit-shimmer absolute inset-0" />
          </div>
          <div className="flex flex-1 flex-col gap-2">
            <SkeletonLine width="45%" />
            <SkeletonLine width="70%" />
          </div>
        </div>
      ))}
    </div>
  )
}
