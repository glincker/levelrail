export const CARD_GRID_CLASS =
  'grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4'

export function AppCardSkeleton() {
  return (
    <li
      aria-hidden="true"
      className="overflow-hidden rounded-xl border border-border bg-card"
    >
      <div className="h-32 animate-pulse bg-muted motion-reduce:animate-none" />
      <div className="space-y-2 p-3">
        <div className="h-4 w-2/3 animate-pulse rounded bg-muted motion-reduce:animate-none" />
        <div className="h-3 w-1/2 animate-pulse rounded bg-muted motion-reduce:animate-none" />
      </div>
    </li>
  )
}
