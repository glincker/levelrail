import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { routeErrorMessage } from '../../lib/apiError'
import { databaseListQueryOptions } from '../../queries/databases'
import {
  DatabasesError,
  DatabasesPage,
  DatabasesPending,
} from '../../components/DatabasesPage'

// Loader primes the cache; the component reads it via useSuspenseQuery.
// The table pages client-side: GET /api/v1/databases returns everything.
export const Route = createFileRoute('/databases/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(databaseListQueryOptions()),
  validateSearch: (search: Record<string, unknown>): { create?: string } =>
    search.create === '1' || search.create === 1 ? { create: '1' } : {},
  component: DatabaseListPage,
  pendingComponent: DatabasesPending,
  errorComponent: DatabaseListError,
})

function DatabaseListPage() {
  useEnvironmentScope()
  const { data: databases } = useSuspenseQuery(databaseListQueryOptions())
  return <DatabasesPage databases={databases} />
}

function DatabaseListError({ error }: { error: unknown }) {
  return <DatabasesError message={routeErrorMessage(error)} />
}
