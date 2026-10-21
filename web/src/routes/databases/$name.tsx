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
import { StuckPanel } from '../../components/StuckPanel'
import { DatabaseActionBar } from '../../components/databases/DatabaseActionBar'
import { DatabasePaletteActions } from '../../components/databases/DatabasePaletteActions'
import {
  parseDatabaseAction,
  type DatabasePaletteAction,
} from '../../lib/paletteAction'
import { DeleteExternalDatabaseDialog } from '../../components/ExternalDatabaseOverview'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'

// Layout route: primes the database and status queries once so the section
// routes and sidebar read the same cache.
export const Route = createFileRoute('/databases/$name')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    Promise.all([
      queryClient.ensureQueryData(databaseDetailQueryOptions(name)),
      queryClient.ensureQueryData(databaseStatusQueryOptions(name)),
    ]),
  validateSearch: (
    search: Record<string, unknown>,
  ): { action?: DatabasePaletteAction } => {
    const action = parseDatabaseAction(search.action)
    return action ? { action } : {}
  },
  component: DatabaseDetailLayout,
  pendingComponent: DatabaseDetailSkeleton,
  errorComponent: DatabaseDetailError,
})

function DatabaseDetailLayout() {
  const { name } = Route.useParams()
  const { action } = Route.useSearch()
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  const status = summarizeDatabaseStatus(conditions)

  return (
    <PageTemplate
      title={database.name}
      breadcrumb={
        <Breadcrumbs projectId={database.project_id} page={database.name} />
      }
      status={<Badge variant={status.variant}>{status.label}</Badge>}
      actions={
        database.external ? (
          <DeleteExternalDatabaseDialog name={database.name} />
        ) : (
          <DatabaseActionBar database={database} />
        )
      }
    >
      {database.external ? null : (
        <DatabasePaletteActions database={database} action={action} />
      )}
      {database.external ? null : (
        <StuckPanel kind="database" name={database.name} />
      )}
      <Outlet />
    </PageTemplate>
  )
}

// Mirrors this layout's real header plus routes/databases/$name/overview.tsx's
// own card stack (Overview dl, conditions, public access, backups, restore),
// since the loader here blocks that whole outlet, not just the header.
function DatabaseDetailSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <Skeleton className="h-4 w-40" />
      <div className="flex flex-wrap items-center justify-between gap-3">
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
  const { t } = useTranslation('databases')
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
        <Link to="/databases" className="mt-2 inline-block underline">
          {t('detail.backToList')}
        </Link>
      </AlertDescription>
    </Alert>
  )
}
