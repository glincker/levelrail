import { createFileRoute } from '@tanstack/react-router'
import { nodePagesQueryOptions } from '../../queries/nodes'
import {
  NodeListPage,
  NodeListPending,
} from '../../components/nodes/NodesListPage'

// The loader primes the cache; the table virtualizes itself past 50 rows.
export const Route = createFileRoute('/nodes/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureInfiniteQueryData(nodePagesQueryOptions()),
  component: NodeListPage,
  pendingComponent: NodeListPending,
})
