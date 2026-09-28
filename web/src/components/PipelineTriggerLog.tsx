import { Link } from '@tanstack/react-router'
import { Badge } from '@/components/ui/badge'
import { usePipelineTriggers } from '../queries/pipelines'
import type { PipelineTriggerDecision } from '../types/pipelines'
import type { BadgeVariant } from '../lib/pipelineStatus'

const DECISION_VARIANT: Record<
  PipelineTriggerDecision['decision'],
  BadgeVariant
> = {
  started: 'success',
  held: 'warning',
  skipped: 'muted',
  failed: 'destructive',
}

// Why recent pushes, tags, and pull requests did or did not start a run, so
// "my push did nothing" has an answer without reading server logs.
export function PipelineTriggerLog({ appName }: { appName: string }) {
  const { data, error } = usePipelineTriggers(appName)
  if (error || !data || data.length === 0) {
    return null
  }
  return (
    <section className="rounded-lg border border-border p-4">
      <h2 className="text-sm font-semibold text-foreground">Recent triggers</h2>
      <p className="mt-1 text-xs text-muted-foreground">
        What happened to recent git events, including the ones that started
        nothing.
      </p>
      <ul className="mt-3 divide-y divide-border text-sm">
        {data.map((t) => (
          <li key={t.id} className="flex flex-wrap items-center gap-2 py-1.5">
            <Badge variant={DECISION_VARIANT[t.decision]}>{t.decision}</Badge>
            <span className="text-xs text-muted-foreground">
              {t.event.replace('_', ' ')}
              {t.pipeline ? ` / ${t.pipeline}` : ''}
            </span>
            <span className="min-w-48 flex-1 text-foreground">{t.reason}</span>
            {t.run_id ? (
              <Link
                to="/apps/$name/pipelines/runs/$runId"
                params={{ name: appName, runId: t.run_id }}
                className="text-xs underline underline-offset-2"
              >
                View run
              </Link>
            ) : null}
            <time
              dateTime={t.created_at}
              className="text-xs tabular-nums text-muted-foreground"
            >
              {new Date(t.created_at).toLocaleString()}
            </time>
          </li>
        ))}
      </ul>
    </section>
  )
}
