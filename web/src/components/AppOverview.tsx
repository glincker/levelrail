import { SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail, ServiceProbe } from '../types/appDetail'
import { formatBytes, formatDurationNs, formatNanoCpus } from '../lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { MoveToNodeDialog } from './MoveToNodeDialog'

// Read-only display of an app's current desired state: image, port,
// resource limits, and health probe config, straight from GET
// /api/v1/apps/{name}. Domains and env are editable elsewhere
// (DomainEditor, EnvEditor); everything shown here has no edit affordance
// on this pass, matching TASKS.md 1.10's scope for this route.
export function AppOverview({ app }: { app: AppDetail }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SquaresFourIcon className="size-4" />
          Overview
        </CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
          <Field label="Image" value={app.image} mono />
          <Field label="Port" value={String(app.port)} />
          <Field
            label="Memory limit"
            value={formatBytes(app.resources?.memory_bytes)}
          />
          <Field
            label="CPU limit"
            value={formatNanoCpus(app.resources?.nano_cpus)}
          />
          {/* Same fallback convention routes/databases/$name.tsx uses for
              the equivalent field: an italic "unassigned" placeholder
              rather than blank space, so an app with no explicit
              placement reads as a deliberate state, not missing data. */}
          <div>
            <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Node
            </dt>
            <dd className="mt-0.5 flex items-center gap-2 font-mono text-sm text-foreground">
              {app.node_id || (
                <span className="text-muted-foreground italic">unassigned</span>
              )}
              <MoveToNodeDialog
                kind="app"
                name={app.name}
                currentNodeId={app.node_id}
              />
            </dd>
          </div>
        </dl>
        <div className="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <ProbeSummary label="Readiness probe" probe={app.health?.readiness} />
          <ProbeSummary label="Liveness probe" probe={app.health?.liveness} />
        </div>
      </CardContent>
    </Card>
  )
}

function Field({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div>
      <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </dt>
      <dd
        className={`mt-0.5 text-sm text-foreground ${mono ? 'font-mono' : ''}`}
      >
        {value}
      </dd>
    </div>
  )
}

function ProbeSummary({
  label,
  probe,
}: {
  label: string
  probe?: ServiceProbe | null
}) {
  return (
    <div className="rounded-md bg-muted p-3">
      <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </p>
      {probe ? (
        <ul className="mt-1.5 space-y-0.5 text-sm text-foreground/90">
          <li>
            Path: <span className="font-mono">{probe.path}</span>
          </li>
          <li>Interval: {formatDurationNs(probe.interval)}</li>
          <li>Timeout: {formatDurationNs(probe.timeout)}</li>
          <li>Failure threshold: {probe.failures ?? 'not set'}</li>
        </ul>
      ) : (
        <p className="mt-1.5 text-sm text-muted-foreground">not configured</p>
      )}
    </div>
  )
}
