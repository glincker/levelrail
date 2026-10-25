import { createFileRoute } from '@tanstack/react-router'
import { DomainManagedByStackBanner } from '../../components/stacks/ManagedByStackNotice'
import { StuckPanel } from '../../components/StuckPanel'
import { DomainPolicyPage } from '../../components/domain/DomainPolicyPage'
import { validateDomainSearch } from '../../components/domain/domainSearch'
import { domainPoliciesQueryOptions } from '../../queries/domainPolicies'

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
  const { app, tab, fix } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <div className="space-y-4">
      <DomainManagedByStackBanner domain={domain} />
      {app ? <StuckPanel kind="domain" name={domain} appName={app} /> : null}
      <DomainPolicyPage
        app={app}
        domain={domain}
        tab={tab ?? 'overview'}
        fix={fix}
        onTab={(next) => {
          void navigate({ search: { app, tab: next }, replace: true })
        }}
      />
    </div>
  )
}
