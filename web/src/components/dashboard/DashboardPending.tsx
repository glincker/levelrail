import { SkeletonLine, SkeletonTile } from '@/components/kit'

/** One skeleton shared by the route pending state and the in-page loading state so the layout does not jump. */
export function DashboardPending() {
  return (
    <div className="space-y-8" aria-busy="true">
      <SkeletonLine width={260} className="h-6" />
      <div className="grid grid-cols-[repeat(auto-fit,minmax(10rem,1fr))] gap-3">
        {Array.from({ length: 5 }, (_, i) => (
          <SkeletonTile key={i} />
        ))}
      </div>
      <SkeletonTile className="h-40" />
    </div>
  )
}
