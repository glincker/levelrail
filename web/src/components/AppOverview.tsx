import { SquaresFourIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import type {
  AppDetail,
  DeployStrategy,
  ServiceProbe,
} from '../types/appDetail'
import { formatBytes, formatDurationNs, formatNanoCpus } from '../lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { MoveToNodeDialog } from './MoveToNodeDialog'
import { MoveToProjectDialog } from './MoveToProjectDialog'
import { MoveToEnvironmentDialog } from './MoveToEnvironmentDialog'
import { useProjectListOptional } from '../queries/projects'
import { useEnvironmentListOptional } from '../queries/environments'

// Display labels for internal/spec's three strategy constants. "rolling"
// gets a label of its own rather than being title-cased generically,
// since it's the one value worth calling out as a problem the moment it's
// read, not just a name: see the inline warning below.
const STRATEGY_LABELS: Record<DeployStrategy, string> = {
  recreate: 'Recreate',
  'blue-green': 'Blue-green',
  rolling: 'Rolling',
}

// Read-only display of an app's current desired state: image, port,
// resource limits, health probe config, and (as of this pass) deploy
// strategy and replica count, straight from GET /api/v1/apps/{name}.
// Domains and env are editable elsewhere (DomainEditor, EnvEditor); port
// and strategy/replicas are editable via PortEditor/DeployStrategyEditor,
// rendered as sibling cards by the overview route rather than inline
// here, matching how ResourceLimitsEditor/HealthCheckEditor already sit
// next to this read-only card rather than inside it.
export function AppOverview({ app }: { app: AppDetail }) {
  // Optional convenience only, see useProjectListOptional's own doc
  // comment: resolving project_id to a human-readable name is a display
  // nicety, a failed/slow fetch just means the raw id is shown instead.
  const projectList = useProjectListOptional()
  const projectName = projectList.data?.find(
    (p) => p.id === app.project_id,
  )?.name
  // Environments are scoped to a project (internal/api/environments.go),
  // so this list is only meaningful once app.project_id is set; an
  // empty projectId disables the underlying query, same reasoning
  // MoveToEnvironmentDialog's own doc comment gives.
  const environmentList = useEnvironmentListOptional(app.project_id ?? '')
  const environmentName = environmentList.data?.find(
    (e) => e.id === app.environment_id,
  )?.name

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
          <div>
            <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Deploy strategy
            </dt>
            <dd className="mt-0.5 text-sm text-foreground">
              {STRATEGY_LABELS[app.strategy] ?? app.strategy}
              {app.strategy === 'rolling' ? (
                <span className="mt-1 flex items-center gap-1 text-xs text-destructive">
                  <WarningIcon className="size-3.5" />
                  Not supported by the reconciler; the next deploy will fail
                  until this is changed.
                </span>
              ) : null}
            </dd>
          </div>
          <Field label="Replicas" value={String(app.replicas)} />
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
          {/* Project membership is metadata, the same "shown, not a
              filter" role node placement above already has: an app with
              no project stays a real, permanent, equally-valid state,
              never hidden or forced. */}
          <div>
            <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Project
            </dt>
            <dd className="mt-0.5 flex items-center gap-2 text-sm text-foreground">
              {app.project_id ? (
                (projectName ?? app.project_id)
              ) : (
                <span className="text-muted-foreground italic">no project</span>
              )}
              <MoveToProjectDialog
                kind="app"
                name={app.name}
                currentProjectId={app.project_id}
              />
            </dd>
          </div>
          {/* Same "shown, not a filter" metadata role Project has above:
              an app with no environment stays a real, permanent, equally
              valid state. */}
          <div>
            <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Environment
            </dt>
            <dd className="mt-0.5 flex items-center gap-2 text-sm text-foreground">
              {app.environment_id ? (
                (environmentName ?? app.environment_id)
              ) : (
                <span className="text-muted-foreground italic">
                  no environment
                </span>
              )}
              <MoveToEnvironmentDialog
                appName={app.name}
                projectId={app.project_id}
                currentEnvironmentId={app.environment_id}
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
