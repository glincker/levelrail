import { createFileRoute } from '@tanstack/react-router'
import { useDatabase, useDatabaseStatus } from '../../../queries/databases'
import { backupTargetListQueryOptions } from '../../../queries/backupTargets'
import { ConditionsPanel } from '../../../components/ConditionsPanel'
import { MoveToNodeDialog } from '../../../components/MoveToNodeDialog'
import { MoveToProjectDialog } from '../../../components/MoveToProjectDialog'
import { BackupsSection } from '../../../components/BackupsSection'
import { DatabasePublicAccessCard } from '../../../components/DatabasePublicAccessCard'
import { useProjectListOptional } from '../../../queries/projects'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// The one real section Databases has today (engine/version/node summary,
// reconcile status, and this database's own backups), formerly rendered
// directly inside routes/databases/$name.tsx before that file became the
// layout route. Reads database/conditions from the same query cache the
// parent layout route's loader already primed (queries/databases.ts), no
// fetch of its own, mirroring routes/apps/$name/overview.tsx.
//
// This route's own loader additionally primes backupTargetListQueryOptions
// (queries/backupTargets.ts): BackupsSection's target picker reads it via
// useBackupTargets, a useSuspenseQuery call, and there is no ambient
// <Suspense> boundary anywhere above this route to catch an unprimed one,
// the same reason every other useSuspenseQuery call in this codebase has
// a matching loader.
export const Route = createFileRoute('/databases/$name/overview')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(backupTargetListQueryOptions()),
  component: OverviewSection,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  // Optional convenience only, see useProjectListOptional's own doc
  // comment: resolving project_id to a name is a display nicety.
  const projectList = useProjectListOptional()
  const projectName = projectList.data?.find(
    (p) => p.id === database.project_id,
  )?.name

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Overview</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                Engine
              </dt>
              <dd className="mt-1 text-sm font-medium text-foreground capitalize">
                {database.engine}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                Version
              </dt>
              <dd className="mt-1 font-mono text-sm text-foreground">
                {database.version}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">Node</dt>
              <dd className="mt-1 flex items-center gap-2 font-mono text-sm text-foreground">
                {database.node_id || (
                  <span className="text-muted-foreground italic">
                    unassigned
                  </span>
                )}
                <MoveToNodeDialog
                  kind="database"
                  name={database.name}
                  currentNodeId={database.node_id}
                />
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                Project
              </dt>
              <dd className="mt-1 flex items-center gap-2 text-sm text-foreground">
                {database.project_id ? (
                  (projectName ?? database.project_id)
                ) : (
                  <span className="text-muted-foreground italic">
                    no project
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

      <ConditionsPanel conditions={conditions} />

      <DatabasePublicAccessCard database={database} />

      <BackupsSection databaseName={database.name} />
    </div>
  )
}
