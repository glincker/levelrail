import { createFileRoute } from '@tanstack/react-router'
import { DomainPolicyPage } from '../../components/domain/DomainPolicyPage'
import { DOMAIN_TABS, type DomainTab } from '../../components/domain/domainTabs'
import { domainPoliciesQueryOptions } from '../../queries/domainPolicies'

interface DomainSearch {
  app: string
  tab?: DomainTab
}

// Plain function rather than zod so validateSearch stays out of the eagerly loaded bundle.
function validateDomainSearch(search: Record<string, unknown>): DomainSearch {
  const app = typeof search.app === 'string' ? search.app : ''
  const tab = DOMAIN_TABS.find((t) => t === search.tab)
  return tab ? { app, tab } : { app }
}

export const Route = createFileRoute('/domains/$domain')({
  validateSearch: validateDomainSearch,
  loaderDeps: ({ search }) => ({ app: search.app }),
  loader: ({ context: { queryClient }, params, deps }) =>
    deps.app
      ? queryClient
          .ensureQueryData(domainPoliciesQueryOptions(deps.app, params.domain))
          .catch(() => undefined)
      : undefined,
  component: DomainRoute,
})

function DomainRoute() {
  const { domain } = Route.useParams()
  const { app, tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <DomainPolicyPage
      app={app}
      domain={domain}
      tab={tab ?? 'overview'}
      onTab={(next) => {
        void navigate({ search: { app, tab: next }, replace: true })
      }}
    />
  )
}
