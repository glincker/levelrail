import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { requireExperimental } from '../../../lib/experimental'
import { routeErrorMessage } from '../../../lib/apiError'
import { projectDetailQueryOptions } from '../../../queries/projects'
import { projectTopologyQueryOptions } from '../../../queries/projectTopology'
import { CanvasPage } from '../../../components/canvas/CanvasPage'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'

// Beta: the guard runs before the loader so a switched-off Canvas fires no project requests.
export const Route = createFileRoute('/projects/$id/canvas')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'canvas', location.href),
  loader: ({ context: { queryClient }, params: { id } }) =>
    Promise.all([
      queryClient.ensureQueryData(projectDetailQueryOptions(id)),
      queryClient.ensureQueryData(projectTopologyQueryOptions(id)),
    ]),
  component: CanvasRoute,
  pendingComponent: CanvasPending,
  errorComponent: CanvasError,
})

function CanvasRoute() {
  const { id } = Route.useParams()
  return <CanvasPage projectId={id} />
}

function CanvasPending() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <Skeleton className="h-6 w-56" />
      <Skeleton className="h-[28rem] w-full rounded-xl" />
    </div>
  )
}

function CanvasError({ error }: { error: unknown }) {
  const { t } = useTranslation('canvas')
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p className="font-medium">{t('loadError')}</p>
        <p>{routeErrorMessage(error)}</p>
      </AlertDescription>
    </Alert>
  )
}
