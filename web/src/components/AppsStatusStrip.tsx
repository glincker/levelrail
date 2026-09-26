import { useQuery } from '@tanstack/react-query'
import { appsSummaryQueryOptions } from '../queries/appsBulk'

const CELLS = [
  { key: 'running', label: 'Running', dot: 'bg-emerald-500' },
  { key: 'deploying', label: 'Deploying', dot: 'bg-amber-500' },
  { key: 'failing', label: 'Failing', dot: 'bg-destructive' },
  { key: 'stopped', label: 'Stopped', dot: 'bg-muted-foreground/50' },
] as const

// Counts come from GET /api/v1/apps-summary (names plus one batched
// conditions query), so the strip stays cheap at hundreds of apps.
export function AppsStatusStrip() {
  const { data } = useQuery(appsSummaryQueryOptions())
  if (!data || data.total === 0) {
    return null
  }
  return (
    <ul
      className="mb-3 flex flex-wrap items-center gap-x-5 gap-y-1 text-sm"
      aria-label="App status summary"
    >
      {CELLS.map((cell) => (
        <li key={cell.key} className="flex items-center gap-1.5">
          <span
            className={`size-2 rounded-full ${cell.dot}`}
            aria-hidden="true"
          />
          <span className="font-medium text-foreground">{data[cell.key]}</span>
          <span className="text-muted-foreground">{cell.label}</span>
        </li>
      ))}
    </ul>
  )
}
