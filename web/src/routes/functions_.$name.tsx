import { createFileRoute } from '@tanstack/react-router'
import { FunctionDetail } from '../components/functions/FunctionDetail'
import { appDetailQueryOptions } from '../queries/apps'

// Trailing underscore keeps this page out of the /functions list layout.
export const Route = createFileRoute('/functions_/$name')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(appDetailQueryOptions(name)),
  component: FunctionDetailRoute,
})

function FunctionDetailRoute() {
  const { name } = Route.useParams()
  return <FunctionDetail name={name} />
}
