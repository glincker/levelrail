import { lazy, Suspense } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useDatabase, useDatabaseStatus } from '../../../queries/databases'
import { ConditionsPanel } from '../../../components/ConditionsPanel'
import { PlacementCard } from '../../../components/PlacementCard'
import {
  KeyFacts,
  ServiceSummary,
} from '../../../components/databases/home/OverviewSummary'
import { BackupsSection } from '../../../components/BackupsSection'
import { NoBackupsCard } from '../../../components/databases/NoBackupsCard'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { isCacheEngine } from '@/lib/cacheAccess'
import { PointInTimeRestoreSection } from '../../../components/PointInTimeRestoreSection'
import { MajorUpgradeCard } from '../../../components/MajorUpgradeCard'
import { DatabasePublicAccessCard } from '../../../components/DatabasePublicAccessCard'
import { DatabasePoolerCard } from '../../../components/DatabasePoolerCard'
import { useProjectListOptional } from '../../../queries/projects'
import { ExternalDatabaseOverview } from '../../../components/ExternalDatabaseOverview'
import { Card, CardContent } from '@/components/ui/card'
import { ResourceLabelsCard } from '../../../components/ResourceLabelsCard'
import { DatabaseLinksCard } from '../../../components/databases/DatabaseLinksCard'
const ConnectionCard = lazy(() =>
  import('../../../components/databases/overview/ConnectionCard').then((m) => ({
    default: m.ConnectionCard,
  })),
)
const CacheConnectionCard = lazy(() =>
  import('../../../components/databases/overview/CacheConnectionCard').then(
    (m) => ({ default: m.CacheConnectionCard }),
  ),
)
const DatabaseDataActionsCard = lazy(() =>
  import('../../../components/databases/DatabaseDataActionsCard').then((m) => ({
    default: m.DatabaseDataActionsCard,
  })),
)
const DatabaseParamsCard = lazy(() =>
  import('../../../components/databases/DatabaseParamsCard').then((m) => ({
    default: m.DatabaseParamsCard,
  })),
)
const ProvisioningPanel = lazy(() =>
  import('../../../components/databases/ProvisioningPanel').then((m) => ({
    default: m.ProvisioningPanel,
  })),
)
import { PageToc, type TocItem } from '@/components/PageToc'
import { useTranslation } from 'react-i18next'
import { DatabaseWiring } from '../../../components/databases/overview/DatabaseWiring'
import { StatusHeader } from '../../../components/databases/overview/StatusHeader'
import { Skeleton } from '@/components/ui/skeleton'
import { OptimiseHint } from '../../../components/resourceAdvisor/ResourceAdvisorTile'

// Reads database and conditions from the cache the layout route primed.
export const Route = createFileRoute('/databases/$name/overview')({
  component: OverviewSection,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  const { t } = useTranslation('dbOverview')
  // Optional convenience only, see useProjectListOptional's own doc
  // comment: resolving project_id to a name is a display nicety.
  const projectList = useProjectListOptional()
  const projectName = projectList.data?.find(
    (p) => p.id === database.project_id,
  )?.name

  const toc: TocItem[] = [
    'wiring',
    'details',
    'labels',
    'connection',
    'access',
    'backups',
    'placement',
    'status',
    ...(database.engine === 'postgres' ? ['upgrade'] : []),
  ].map((id) => ({
    id,
    label: t(`toc.sections.${id}` as 'toc.sections.wiring'),
  }))

  if (database.external) {
    return <ExternalDatabaseOverview db={database.external} />
  }

  return (
    <div className="space-y-6">
      <PageToc items={toc} />
      <OptimiseHint kind="database" name={name} />
      <Suspense fallback={null}>
        <ProvisioningPanel name={name} hideWhenReady />
      </Suspense>
      <StatusHeader database={database} conditions={conditions} />
      <div id="wiring" className="scroll-mt-20">
        <DatabaseWiring
          database={database}
          conditions={conditions}
          projectName={projectName}
        />
      </div>
      <div id="labels" className="scroll-mt-20">
        <ResourceLabelsCard path="databases" name={name} />
      </div>
      <div
        id="details"
        className="grid scroll-mt-20 items-start gap-4 lg:grid-cols-[1fr_18rem]"
      >
        <ServiceSummary database={database} />
        <KeyFacts database={database} projectName={projectName} />
      </div>

      <div id="connection" className="scroll-mt-20 space-y-6">
        {database.provision_state ? null : (
          <Suspense
            fallback={
              <Card aria-hidden="true">
                <CardContent className="space-y-3">
                  <Skeleton className="h-6 w-48" />
                  <Skeleton className="h-10 w-full" />
                </CardContent>
              </Card>
            }
          >
            <ConnectionCard database={database} />
          </Suspense>
        )}
        {!database.provision_state && isCacheEngine(database.engine) ? (
          <Suspense fallback={null}>
            <CacheConnectionCard database={database} />
          </Suspense>
        ) : null}
        <DatabaseLinksCard database={database.name} />
        <Suspense fallback={null}>
          <DatabaseDataActionsCard database={database.name} />
        </Suspense>
      </div>

      <div id="access" className="scroll-mt-20 space-y-6">
        <DatabasePublicAccessCard database={database} />
        <DatabasePoolerCard database={database} />
        <Suspense fallback={null}>
          <DatabaseParamsCard database={database.name} />
        </Suspense>
      </div>

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

      <div id="placement" className="scroll-mt-20">
        <PlacementCard
          kind="database"
          name={database.name}
          currentNodeId={database.node_id}
          currentNodeName={database.node_name}
          currentNodeIsLocal={database.node_is_local}
        />
      </div>

      <div id="status" className="scroll-mt-20">
        <ConditionsPanel conditions={conditions} />
      </div>

      {database.engine === 'postgres' ? (
        <div id="upgrade" className="scroll-mt-20">
          <MajorUpgradeCard database={database} />
        </div>
      ) : null}
    </div>
  )
}
