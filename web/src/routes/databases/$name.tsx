import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeftIcon } from 'lucide-react'
import {
  databaseDetailQueryOptions,
  databaseStatusQueryOptions,
  useDatabase,
  useDatabaseStatus,
} from '../../queries/databases'
import type { ReconcileCondition } from '../../types/deploy'
import { ConditionsPanel } from '../../components/ConditionsPanel'
import { DeleteDatabaseDialog } from '../../components/DeleteDatabaseDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { VariantProps } from 'class-variance-authority'

// Database detail route, mirroring routes/apps/$name.tsx's shape. Two
// queries are primed in the loader in parallel, matching that route's
// own reasoning: the database resource itself (GET
// /api/v1/databases/{name}) and its current reconcile status (GET
// /api/v1/databases/{name}/status). Both are read via useSuspenseQuery
// under the hood (useDatabase, useDatabaseStatus), so the component
// below never renders a loading state of its own, the loader already
// guarantees the cache is warm.
export const Route = createFileRoute('/databases/$name')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    Promise.all([
      queryClient.ensureQueryData(databaseDetailQueryOptions(name)),
      queryClient.ensureQueryData(databaseStatusQueryOptions(name)),
    ]),
  component: DatabaseDetailPage,
  errorComponent: DatabaseDetailError,
})

// Same summary rollup AppDetail's own summarizeConditions does, kept as
// a direct copy rather than a shared import: both are small, route-local
// display helpers over the same ReconcileCondition[] shape, and a shared
// util would be one more indirection for four lines of logic. Mirrors
// how Coolify/Dokploy show a single status pill next to the resource
// name at the top of the detail page.
function summarizeConditions(conditions: ReconcileCondition[]): {
  label: string
  variant: VariantProps<typeof badgeVariants>['variant']
} {
  if (conditions.length === 0) {
    return { label: 'No status yet', variant: 'muted' }
  }
  if (conditions.some((c) => c.Status === 'False')) {
    return { label: 'Attention needed', variant: 'destructive' }
  }
  if (conditions.every((c) => c.Status === 'True')) {
    return { label: 'Healthy', variant: 'success' }
  }
  return { label: 'Reconciling', variant: 'muted' }
}

function DatabaseDetailPage() {
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  const status = summarizeConditions(conditions)

  return (
    <div className="space-y-6">
      <div>
        <Link
          to="/databases"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3" />
          Databases
        </Link>
        <div className="mt-1 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <h1 className="text-lg font-semibold text-foreground">
              {database.name}
            </h1>
            <Badge variant={status.variant}>{status.label}</Badge>
          </div>
          <DeleteDatabaseDialog name={database.name} />
        </div>
      </div>

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
              <dd className="mt-1 font-mono text-sm text-foreground">
                {database.node_id || (
                  <span className="text-muted-foreground italic">
                    unassigned
                  </span>
                )}
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <ConditionsPanel conditions={conditions} />
    </div>
  )
}

// fetchDatabase (queries/databases.ts) throws a plain ApiError for a
// 404, which lands here rather than crashing the whole route tree,
// mirroring AppDetailError.
function DatabaseDetailError({ error }: { error: Error }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{error.message}</p>
        <Link to="/databases" className="mt-2 inline-block underline">
          Back to databases
        </Link>
      </AlertDescription>
    </Alert>
  )
}
