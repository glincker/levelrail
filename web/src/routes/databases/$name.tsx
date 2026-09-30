import { createFileRoute, Link, Outlet } from '@tanstack/react-router'
import {
  databaseDetailQueryOptions,
  databaseStatusQueryOptions,
  useDatabase,
  useDatabaseStatus,
} from '../../queries/databases'
import { summarizeDatabaseStatus } from '../../lib/databaseStatus'
import { routeErrorMessage } from '../../lib/apiError'
import { Breadcrumbs } from '../../components/Breadcrumbs'
import { DeleteDatabaseDialog } from '../../components/DeleteDatabaseDialog'
import { StopStartDatabaseButton } from '../../components/StopStartDatabaseButton'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// Database detail layout route, mirroring routes/apps/$name.tsx's own
// split (the Databases fast-follow to that same treatment): this file
// used to render the entire
// page itself (header, Overview card, ConditionsPanel). It is now the
// layout: it owns the loader (both queries below) and the shared page
// header (database name, status, delete action), then renders <Outlet />
// for whichever section route (overview.tsx, resources.tsx) is active.
// Unlike apps/$name.tsx there is no pinned deploy-trigger form here:
// databases have no equivalent per-resource action that applies across
// every section.
//
// Both queries are primed here, matching apps/$name.tsx's own reasoning:
// the database resource itself (GET /api/v1/databases/{name}) and its
// current reconcile status (GET /api/v1/databases/{name}/status). The
// section route and DatabaseScopedSidebar both read the same cache via
// useDatabase/useDatabaseStatus (queries/databases.ts), so navigating
// never re-fetches either query.
export const Route = createFileRoute('/databases/$name')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    Promise.all([
      queryClient.ensureQueryData(databaseDetailQueryOptions(name)),
      queryClient.ensureQueryData(databaseStatusQueryOptions(name)),
    ]),
  component: DatabaseDetailLayout,
  pendingComponent: DatabaseDetailSkeleton,
  errorComponent: DatabaseDetailError,
})

function DatabaseDetailLayout() {
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  const status = summarizeDatabaseStatus(conditions)

  return (
    <div className="space-y-6">
      <div>
        <Breadcrumbs projectId={database.project_id} page={database.name} />
      </div>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h1 className="text-lg font-semibold text-foreground">
            {database.name}
          </h1>
          <Badge variant={status.variant}>{status.label}</Badge>
        </div>
        <div className="flex items-center gap-2">
          <StopStartDatabaseButton
            name={database.name}
            suspended={Boolean(database.suspended)}
          />
          <DeleteDatabaseDialog name={database.name} />
        </div>
      </div>

      <Outlet />
    </div>
  )
}

// Mirrors this layout's real header plus routes/databases/$name/overview.tsx's
// own card stack (Overview dl, conditions, public access, backups, restore),
// since the loader here blocks that whole outlet, not just the header.
function DatabaseDetailSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <Skeleton className="h-4 w-40" />
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Skeleton className="h-6 w-40" />
          <Skeleton className="h-5 w-16 rounded-full" />
        </div>
        <div className="flex items-center gap-2">
          <Skeleton className="h-9 w-20 rounded-md" />
          <Skeleton className="h-9 w-20 rounded-md" />
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>
            <Skeleton className="h-4 w-20" />
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            {Array.from({ length: 5 }, (_, i) => (
              <div key={i} className="space-y-1">
                <Skeleton className="h-3 w-16" />
                <Skeleton className="h-4 w-24" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {Array.from({ length: 3 }, (_, i) => (
        <Card key={i}>
          <CardHeader>
            <CardTitle>
              <Skeleton className="h-4 w-32" />
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-2/3" />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

// fetchDatabase (queries/databases.ts) throws a plain ApiError for a
// 404, which lands here rather than crashing the whole route tree,
// mirroring AppDetailError.
function DatabaseDetailError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
        <Link to="/databases" className="mt-2 inline-block underline">
          Back to databases
        </Link>
      </AlertDescription>
    </Alert>
  )
}
