import { createFileRoute } from '@tanstack/react-router'
import { StackIcon, RocketIcon } from '@phosphor-icons/react/dist/ssr'
import { appGroupQueryOptions } from '../../../queries/appGroup'
import { AppServicesPanel } from '../../../components/AppServicesPanel'
import { DeploySpecForm } from '../../../components/DeploySpecForm'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { Skeleton } from '@/components/ui/skeleton'

// Safe to visit for a single-service app too, see AppServicesPanel's own comment.
export const Route = createFileRoute('/apps/$name/services')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(appGroupQueryOptions(name)),
  component: ServicesSection,
  pendingComponent: ServicesSectionSkeleton,
})

function ServicesSection() {
  const { name } = Route.useParams()

  return (
    <div className="space-y-6">
      <AppServicesPanel appName={name} />
      <DeploySpecForm appName={name} />
    </div>
  )
}

// Mirrors AppServicesPanel's card+table and DeploySpecForm's card+form
// below it, the two cards this route actually renders.
function ServicesSectionSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <StackIcon className="size-4" />
            Services
          </CardTitle>
          <Skeleton className="h-4 w-64" />
        </CardHeader>
        <CardContent className="space-y-3">
          <Skeleton className="h-5 w-32" />
          <TableSkeleton columnCount={5} rowCount={3} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <RocketIcon className="size-4" />
            Deploy multiple services
          </CardTitle>
          <Skeleton className="h-4 w-72" />
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-3 sm:flex-row">
            <Skeleton className="h-9 flex-[2]" />
            <Skeleton className="h-9 flex-1" />
            <Skeleton className="h-9 flex-1" />
          </div>
          <Skeleton className="h-16 w-full rounded-md" />
          <div className="flex items-center justify-between">
            <Skeleton className="h-8 w-28 rounded-md" />
            <Skeleton className="h-8 w-32 rounded-md" />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
