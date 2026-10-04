import { createFileRoute } from '@tanstack/react-router'
import { PlugsIcon } from '@phosphor-icons/react/dist/ssr'
import { appStreamListQueryOptions } from '../../../queries/appStreams'
import { AppStreamsPanel } from '../../../components/AppStreamsPanel'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// Mirrors domains.tsx's own shape: a real, deep-linkable per-app route,
// its loader priming the query cache this page's suspense query reads.
export const Route = createFileRoute('/apps/$name/streams')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(appStreamListQueryOptions(name)),
  component: StreamsSection,
  pendingComponent: StreamsSectionSkeleton,
})

function StreamsSection() {
  const { name } = Route.useParams()
  return <AppStreamsPanel appName={name} />
}

// Mirrors DomainsSectionSkeleton's own card: header, a form-shaped row,
// then a couple of table-row-shaped skeleton lines.
function StreamsSectionSkeleton() {
  return (
    <Card aria-hidden="true">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <PlugsIcon className="size-4" />
          Streams
        </CardTitle>
        <Skeleton className="h-4 w-72" />
      </CardHeader>
      <CardContent className="space-y-4">
        <Skeleton className="h-16 w-full rounded-lg" />
        {Array.from({ length: 2 }, (_, i) => (
          <Skeleton key={i} className="h-9 w-full rounded-md" />
        ))}
      </CardContent>
    </Card>
  )
}
