import { createFileRoute } from '@tanstack/react-router'
import { useDatabase, useDatabaseStatus } from '../../../queries/databases'
import { ConditionsPanel } from '../../../components/ConditionsPanel'
import { MoveToNodeDialog } from '../../../components/MoveToNodeDialog'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// The one real section Databases has today (engine/version/node summary
// plus reconcile status), formerly rendered directly inside
// routes/databases/$name.tsx before that file became the layout route.
// Reads database/conditions from the same query cache the parent
// layout route's loader already primed (queries/databases.ts), no fetch
// of its own, mirroring routes/apps/$name/overview.tsx.
export const Route = createFileRoute('/databases/$name/overview')({
  component: OverviewSection,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)

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
          </dl>
        </CardContent>
      </Card>

      <ConditionsPanel conditions={conditions} />
    </div>
  )
}
