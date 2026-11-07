import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { OptimiseHint } from './resourceAdvisor/ResourceAdvisorTile'
import type {
  AppDetail,
  DeployStrategy,
  ServiceProbe,
} from '../types/appDetail'
import { formatBytes, formatDurationNs, formatNanoCpus } from '../lib/format'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { FieldLabel } from '@/components/ui/typography'
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
import { GameStatusCard } from './apps/GameStatusCard'
import { TunnelHint } from './apps/TunnelHint'
import { VitalsCard } from './apps/VitalsCard'

// Maps the three backend strategy constants to i18n keys.
const STRATEGY_KEYS = {
  recreate: 'strategy.recreate',
  'blue-green': 'strategy.blueGreen',
  rolling: 'strategy.rolling',
} as const satisfies Record<DeployStrategy, string>

// Read-only summary of an app's desired state from GET /api/v1/apps/{name}.
// Port and strategy editors render as sibling cards on the overview route.
export function AppOverview({ app }: { app: AppDetail }) {
  const { t } = useTranslation('appOverview')
  // Name lookup is a display nicety: on a failed fetch the raw id is shown.
  const projectList = useProjectListOptional()
  const projectName = projectList.data?.find(
    (p) => p.id === app.project_id,
  )?.name
  // Environments are project-scoped; an empty projectId disables the query.
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
          <LayoutGridGlassIcon className="size-5" />
          {t('title')}
        </CardTitle>
        <OptimiseHint kind="app" name={app.name} />
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
          <Field
            label={t('image')}
            value={unpinnedImage(app.image)}
            mono
            digest={app.image_digest || digestOfImage(app.image)}
          />
          <Field label={t('port')} value={String(app.port)} />
          <Field
            label={t('memoryLimit')}
            value={formatBytes(app.resources?.memory_bytes)}
          />
          <Field
            label={t('cpuLimit')}
            value={formatNanoCpus(app.resources?.nano_cpus)}
          />
          <Field
            label={t('swapLimit')}
            value={formatBytes(app.resources?.swap_memory_bytes)}
          />
          <Field
            label={t('cpuPinning')}
            value={app.resources?.cpuset_cpus || t('notSet')}
          />
          <div>
            <FieldLabel as="dt">{t('deployStrategy')}</FieldLabel>
            <dd className="mt-0.5 text-sm text-foreground">
              {STRATEGY_KEYS[app.strategy]
                ? t(STRATEGY_KEYS[app.strategy])
                : app.strategy}
            </dd>
          </div>
          <div>
            <FieldLabel as="dt">{t('replicas')}</FieldLabel>
            <dd className="mt-0.5 text-sm text-foreground">
              {app.replicas}
              {app.replicas > 1 ? (
                <span className="mt-0.5 block text-xs text-muted-foreground">
                  {t('replicaNotice.sameServer', {
                    ns: 'common',
                    replicas: app.replicas,
                  })}
                </span>
              ) : null}
            </dd>
          </div>
          <div>
            <FieldLabel as="dt">{t('node')}</FieldLabel>
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
          <div>
            <FieldLabel as="dt">{t('project')}</FieldLabel>
            <dd className="mt-0.5 flex items-center gap-2 text-sm text-foreground">
              {app.project_id ? (
                (projectName ?? app.project_id)
              ) : (
                <span className="text-muted-foreground italic">
                  {t('noProject')}
                </span>
              )}
              <MoveToProjectDialog
                kind="app"
                name={app.name}
                currentProjectId={app.project_id}
              />
            </dd>
          </div>
          <div>
            <FieldLabel as="dt">{t('environment')}</FieldLabel>
            <dd className="mt-0.5 flex items-center gap-2 text-sm text-foreground">
              {app.environment_id ? (
                (environmentName ?? app.environment_id)
              ) : (
                <span className="text-muted-foreground italic">
                  {t('noEnvironment')}
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
          <ProbeSummary
            label={t('readinessProbe')}
            probe={app.health?.readiness}
          />
          <ProbeSummary
            label={t('livenessProbe')}
            probe={app.health?.liveness}
          />
        </div>
        <GameStatusCard appName={app.name} />
        <VitalsCard appName={app.name} />
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
      <FieldLabel as="dt">{label}</FieldLabel>
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
  const { t } = useTranslation('appOverview')
  return (
    <div className="rounded-md bg-muted p-3">
      <FieldLabel>{label}</FieldLabel>
      {probe ? (
        <ul className="mt-1.5 space-y-0.5 text-sm text-foreground/90">
          <li>
            {t('path')} <span className="font-mono">{probe.path}</span>
          </li>
          <li>
            {t('interval')} {formatDurationNs(probe.interval)}
          </li>
          <li>
            {t('timeout')} {formatDurationNs(probe.timeout)}
          </li>
          <li>
            {t('failureThreshold')} {probe.failures ?? t('notSet')}
          </li>
        </ul>
      ) : (
        <p className="mt-1.5 text-sm text-muted-foreground">
          {t('notConfigured')}
        </p>
      )}
    </div>
  )
}
