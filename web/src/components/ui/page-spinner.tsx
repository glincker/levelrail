import { SpinnerIcon } from '@phosphor-icons/react/dist/ssr'

// Route pendingComponent fallback for detail-shell layouts (app/database/
// node) that render a header plus several unrelated cards: no single
// repeated row shape to skeleton, so a centered spinner stands in for the
// whole page during the loader's pending phase.
export function PageSpinner() {
  return (
    <div
      className="flex h-64 items-center justify-center text-muted-foreground"
      role="status"
      aria-label="Loading"
    >
      <SpinnerIcon className="size-6 animate-spin" />
    </div>
  )
}
