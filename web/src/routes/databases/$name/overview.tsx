import { lazy, Suspense } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useDatabase, useDatabaseStatus } from '../../../queries/databases'
import { ConditionsPanel } from '../../../components/ConditionsPanel'
import { ChangeDatabaseVersionDialog } from '../../../components/ChangeDatabaseVersionDialog'
import { PlacementCard } from '../../../components/PlacementCard'
import {
  MoveToThisServerButton,
  PlacementLabel,
} from '../../../components/PlacementLabel'
import { MoveToProjectDialog } from '../../../components/MoveToProjectDialog'
import { BackupsSection } from '../../../components/BackupsSection'
import { NoBackupsCard } from '../../../components/databases/NoBackupsCard'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { PointInTimeRestoreSection } from '../../../components/PointInTimeRestoreSection'
import { MajorUpgradeCard } from '../../../components/MajorUpgradeCard'
import { DatabasePublicAccessCard } from '../../../components/DatabasePublicAccessCard'
import { useProjectListOptional } from '../../../queries/projects'
import { ExternalDatabaseOverview } from '../../../components/ExternalDatabaseOverview'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DatabaseLinksCard } from '../../../components/databases/DatabaseLinksCard'
const ProvisioningPanel = lazy(() =>
  import('../../../components/databases/ProvisioningPanel').then((m) => ({
    default: m.ProvisioningPanel,
  })),
)
import { ConnectionDetailsCard } from '../../../components/databases/ConnectionDetailsCard'
import { OptimiseHint } from '../../../components/resourceAdvisor/ResourceAdvisorTile'

// Reads database and conditions from the cache the layout route primed.
export const Route = createFileRoute('/databases/$name/overview')({
  component: OverviewSection,
})

function OverviewSection() {
  const { t } = useTranslation('databases')
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  // Optional convenience only, see useProjectListOptional's own doc
  // comment: resolving project_id to a name is a display nicety.
  const projectList = useProjectListOptional()
  const projectName = projectList.data?.find(
    (p) => p.id === database.project_id,
  )?.name

  if (database.external) {
    return <ExternalDatabaseOverview db={database.external} />
  }

  return (
    <div className="space-y-6">
      <OptimiseHint kind="database" name={name} />
      <Suspense fallback={null}>
        <ProvisioningPanel name={name} hideWhenReady />
      </Suspense>
      <Card>
        <CardHeader>
          <CardTitle>{t('overview.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('connection.engine')}
              </dt>
              <dd className="mt-1 text-sm font-medium text-foreground capitalize">
                {database.engine}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('overview.version')}
              </dt>
              <dd className="mt-1 flex items-center gap-2 break-all font-mono text-sm text-foreground">
                {database.version}
                <ChangeDatabaseVersionDialog
                  name={database.name}
                  engine={database.engine}
                  currentVersion={database.version}
                />
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('overview.node')}
              </dt>
              <dd className="mt-1 flex items-center gap-2 break-all font-mono text-sm text-foreground">
                <PlacementLabel
                  nodeId={database.node_id}
                  nodeName={database.node_name}
                  nodeIsLocal={database.node_is_local}
                />
                <MoveToThisServerButton
                  kind="database"
                  name={database.name}
                  nodeId={database.node_id}
                  nodeName={database.node_name}
                  nodeIsLocal={database.node_is_local}
                />
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('overview.project')}
              </dt>
              <dd className="mt-1 flex items-center gap-2 text-sm text-foreground">
                {database.project_id ? (
                  (projectName ?? database.project_id)
                ) : (
                  <span className="text-muted-foreground italic">
                    {t('overview.noProject')}
                  </span>
                )}
                <MoveToProjectDialog
                  kind="database"
                  name={database.name}
                  currentProjectId={database.project_id}
                />
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <ConnectionDetailsCard database={database} />

      <DatabaseLinksCard database={database.name} />

      <DatabasePublicAccessCard database={database} />

      <div id="backups" className="scroll-mt-20 space-y-6">
        {engineHasBackups(database.engine) ? (
          <>
            <BackupsSection database={database} />
            <PointInTimeRestoreSection database={database} />
          </>
        ) : (
          <NoBackupsCard engine={database.engine} />
        )}
      </div>

      <PlacementCard
        kind="database"
        name={database.name}
        currentNodeId={database.node_id}
        currentNodeName={database.node_name}
        currentNodeIsLocal={database.node_is_local}
      />

      <ConditionsPanel conditions={conditions} />

      {database.engine === 'postgres' ? (
        <MajorUpgradeCard database={database} />
      ) : null}
    </div>
  )
}
