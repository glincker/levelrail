import { Skeleton } from './skeleton'

// Shared loading placeholder for small in-page list panels (alert rules,
// scheduled tasks, notify targets, notifications) that currently fall back
// to plain "Loading..." text while their own useQuery is in flight.
export function ListSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-2" aria-hidden="true">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  )
}
