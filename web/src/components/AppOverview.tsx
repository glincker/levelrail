import { SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import { OptimiseHint } from './resourceAdvisor/ResourceAdvisorTile'
import type {
  AppDetail,
  DeployStrategy,
  ServiceProbe,
} from '../types/appDetail'
import { formatBytes, formatDurationNs, formatNanoCpus } from '../lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { MoveToProjectDialog } from './MoveToProjectDialog'
import { MoveToEnvironmentDialog } from './MoveToEnvironmentDialog'
import { MoveEnvironmentDialog } from './environments/MoveEnvironmentDialog'
import { useGlobalEnvironments } from '../queries/globalEnvironments'
import { useExperimentalFeatures } from '../hooks/useExperimental'
import { isFeatureVisible } from '../lib/experimental'
import { useProjectListOptional } from '../queries/projects'
import { MoveToThisServerButton, PlacementLabel } from './PlacementLabel'
import { useEnvironmentListOptional } from '../queries/environments'
import { PinnedDigestBadge } from './PinnedDigestBadge'
import { digestOfImage, unpinnedImage } from '../lib/imageDigest'
import { TunnelHint } from './apps/TunnelHint'

// Display labels for internal/spec's three strategy constants.
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
  const globalEnvironments = isFeatureVisible(
    'global-environments',
    useExperimentalFeatures(),
  )
  const globalList = useGlobalEnvironments(globalEnvironments)
  const environmentName = (
    globalEnvironments ? globalList.data : environmentList.data
  )?.find((e) => e.id === app.environment_id)?.name

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SquaresFourIcon className="size-4" />
          Overview
        </CardTitle>
        <OptimiseHint kind="app" name={app.name} />
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
          <Field
            label="Image"
            value={unpinnedImage(app.image)}
            mono
            digest={app.image_digest || digestOfImage(app.image)}
          />
          <Field label="Port" value={String(app.port)} />
          <Field
            label="Memory limit"
            value={formatBytes(app.resources?.memory_bytes)}
          />
          <Field
            label="CPU limit"
            value={formatNanoCpus(app.resources?.nano_cpus)}
          />
          <Field
            label="Swap limit"
            value={formatBytes(app.resources?.swap_memory_bytes)}
          />
          <Field
            label="CPU pinning"
            value={app.resources?.cpuset_cpus || 'not set'}
          />
          <div>
            <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Deploy strategy
            </dt>
            <dd className="mt-0.5 text-sm text-foreground">
              {STRATEGY_LABELS[app.strategy] ?? app.strategy}
            </dd>
          </div>
          <Field label="Replicas" value={String(app.replicas)} />
          <div>
            <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Node
            </dt>
            <dd className="mt-0.5 flex flex-wrap items-center gap-2 text-sm text-foreground">
              <PlacementLabel
                nodeId={app.node_id}
                nodeName={app.node_name}
                nodeIsLocal={app.node_is_local}
              />
              <MoveToThisServerButton
                kind="app"
                name={app.name}
                nodeId={app.node_id}
                nodeName={app.node_name}
                nodeIsLocal={app.node_is_local}
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
              {globalEnvironments ? (
                <MoveEnvironmentDialog
                  kind="apps"
                  name={app.name}
                  currentEnvironmentId={app.environment_id}
                />
              ) : (
                <MoveToEnvironmentDialog
                  appName={app.name}
                  projectId={app.project_id}
                  currentEnvironmentId={app.environment_id}
                />
              )}
            </dd>
          </div>
        </dl>
        <div className="mt-5 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <ProbeSummary label="Readiness probe" probe={app.health?.readiness} />
          <ProbeSummary label="Liveness probe" probe={app.health?.liveness} />
        </div>
        <TunnelHint
          appName={app.name}
          port={app.port}
          publishedPorts={app.published_ports}
        />
      </CardContent>
    </Card>
  )
}

function Field({
  label,
  value,
  mono,
  digest,
}: {
  label: string
  value: string
  mono?: boolean
  digest?: string
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
        {digest ? <PinnedDigestBadge digest={digest} className="ml-2" /> : null}
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
