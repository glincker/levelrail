import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { routeErrorMessage } from '../../lib/apiError'
import { databaseListQueryOptions } from '../../queries/databases'
import { LabelFilterInput } from '../../components/LabelFilterInput'
import { useLabelMatches } from '../../queries/resourceLabels'
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
  const [labelSelector, setLabelSelector] = useState('')
  const labelMatches = useLabelMatches('databases', labelSelector)
  const shown = labelMatches.data
    ? databases.filter((db) => labelMatches.data.has(db.name))
    : databases
  return (
    <div className="space-y-4">
      <LabelFilterInput
        value={labelSelector}
        onChange={setLabelSelector}
        invalid={labelMatches.isError}
      />
      <DatabasesPage databases={shown} />
    </div>
  )
}

function DatabaseListError({ error }: { error: unknown }) {
  return <DatabasesError message={routeErrorMessage(error)} />
}
