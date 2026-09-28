import { createFileRoute, Link } from '@tanstack/react-router'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { PageSpinner } from '@/components/ui/page-spinner'
import { ModelDetailPage } from '../../components/models/ModelDetailPage'
import { parseModelTab, type ModelTab } from '../../lib/modelPresentation'
import { routeErrorMessage } from '../../lib/apiError'
import { modelDetailQueryOptions } from '../../queries/models'

export const Route = createFileRoute('/models/$name')({
  validateSearch: (search: Record<string, unknown>): { tab: ModelTab } => ({
    tab: parseModelTab(search.tab),
  }),
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(modelDetailQueryOptions(name)),
  component: ModelDetailRoute,
  pendingComponent: PageSpinner,
  errorComponent: ModelDetailError,
})

function ModelDetailRoute() {
  const { name } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ModelDetailPage
      name={name}
      tab={tab}
      onTabChange={(next) => {
        void navigate({ search: { tab: next }, replace: true })
      }}
    />
  )
}

function ModelDetailError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
        <Link to="/models" className="mt-2 inline-block underline">
          Back to AI models
        </Link>
      </AlertDescription>
    </Alert>
  )
}
