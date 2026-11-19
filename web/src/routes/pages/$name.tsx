import { createFileRoute } from '@tanstack/react-router'
import { PageDetail } from '../../components/pages/PageDetail'
import { PAGE_TABS, type PageTab } from '../../components/pages/pageHelpers'
import { appDetailQueryOptions } from '../../queries/apps'
import { certificatesQueryOptions } from '../../queries/certificates'

interface PageSearch {
  tab?: PageTab
  from?: string
}

export const Route = createFileRoute('/pages/$name')({
  validateSearch: (search: Record<string, unknown>): PageSearch => {
    const tab = PAGE_TABS.find((t) => t === search.tab)
    const from =
      typeof search.from === 'string' && search.from.startsWith('/')
        ? search.from.slice(0, 512)
        : undefined
    return {
      ...(tab && tab !== 'overview' ? { tab } : {}),
      ...(from ? { from } : {}),
    }
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
  const { tab, from } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PageDetail
      name={name}
      tab={tab ?? 'overview'}
      redirectFrom={from}
      onAddRedirect={(path) =>
        void navigate({ search: { tab: 'routes', from: path } })
      }
      onTabChange={(next) =>
        void navigate({
          search: next === 'overview' ? {} : { tab: next },
          replace: true,
        })
      }
    />
  )
}
