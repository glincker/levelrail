import { usePipelineSummary } from '../queries/pipelineOverview'
import { formatSuccessRate } from '../lib/pipelineOverview'

function Tile({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: 'bad' | 'warn'
}) {
  const toneClass =
    tone === 'bad'
      ? 'text-destructive'
      : tone === 'warn'
        ? 'text-amber-700 dark:text-amber-300'
        : 'text-foreground'
  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className={`mt-1 text-2xl font-semibold tabular-nums ${toneClass}`}>
        {value}
      </p>
    </div>
  )
}

export function PipelineSummaryTiles() {
  const { data } = usePipelineSummary()
  const waiting = (data?.waiting_approval ?? 0) + (data?.held ?? 0)
  const failed = data?.failed_24h ?? 0
  return (
    <div
      className="grid grid-cols-2 gap-3 lg:grid-cols-4"
      role="group"
      aria-label="Pipeline summary"
    >
      <Tile label="Running" value={data ? String(data.running) : '-'} />
      <Tile
        label="Failed (24h)"
        value={data ? String(failed) : '-'}
        tone={failed > 0 ? 'bad' : undefined}
      />
      <Tile
        label="Waiting approval"
        value={data ? String(waiting) : '-'}
        tone={waiting > 0 ? 'warn' : undefined}
      />
      <Tile
        label="Success rate (24h)"
        value={data ? formatSuccessRate(data.success_rate_24h) : '-'}
      />
    </div>
  )
}
