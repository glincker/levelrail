import { createFileRoute } from '@tanstack/react-router'
import { PageDetail } from '../../components/pages/PageDetail'
import { PAGE_TABS, type PageTab } from '../../components/pages/pageHelpers'
import { appDetailQueryOptions } from '../../queries/apps'
import { certificatesQueryOptions } from '../../queries/certificates'

interface PageSearch {
  tab?: PageTab
}

export const Route = createFileRoute('/pages/$name')({
  validateSearch: (search: Record<string, unknown>): PageSearch => {
    const tab = PAGE_TABS.find((t) => t === search.tab)
    return tab && tab !== 'deployments' ? { tab } : {}
  },
  loader: ({ context: { queryClient }, params: { name } }) =>
    Promise.all([
      queryClient.ensureQueryData(appDetailQueryOptions(name)),
      queryClient.ensureQueryData(certificatesQueryOptions()),
    ]),
  component: PageRoute,
})

function PageRoute() {
  const { name } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PageDetail
      name={name}
      tab={tab ?? 'deployments'}
      onTabChange={(next) =>
        void navigate({
          search: next === 'deployments' ? {} : { tab: next },
          replace: true,
        })
      }
    />
  )
}
