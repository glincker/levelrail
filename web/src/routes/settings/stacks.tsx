import { createFileRoute } from '@tanstack/react-router'
import { requireExperimental } from '../../lib/experimental'
import { StacksPage } from '../../components/stacks/StacksPage'
import { parseStacksSearch, type StacksSearch } from '../../lib/stacksSearch'

// Experimental (APP_EXPERIMENTAL=iac). Selection lives in the URL so a revision or diff can be shared.
export const Route = createFileRoute('/settings/stacks')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'iac', location.href),
  validateSearch: (search: Record<string, unknown>): StacksSearch =>
    parseStacksSearch(search),
  component: StacksRoute,
})

function StacksRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StacksPage
      search={search}
      onSearchChange={(next) => {
        void navigate({ search: next })
      }}
    />
  )
}
