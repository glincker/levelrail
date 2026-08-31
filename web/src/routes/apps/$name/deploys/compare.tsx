import { createFileRoute, Link } from '@tanstack/react-router'
import { GitDiffIcon } from '@phosphor-icons/react/dist/ssr'
import { deployCompareQueryOptions, useDeployCompare } from '../../../../queries/deployCompare'
import { DeployCompareView } from '../../../../components/DeployCompareView'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import { Button } from '@/components/ui/button'

interface DeployCompareSearch {
  from: string
  to?: string
}

// A plain function, not a zod schema: matches routes/reset-password.tsx's
// own reasoning (validateSearch runs as part of eager route matching, so
// pulling zod in here would grow the initial bundle for one pair of
// optional string fields).
function validateDeployCompareSearch(
  search: Record<string, unknown>,
): DeployCompareSearch {
  const { from, to } = search
  return {
    from: typeof from === 'string' ? from : '',
    to: typeof to === 'string' && to !== '' ? to : undefined,
  }
}

// Reached from DeployAttemptsList.tsx's "Compare" selection, either two
// picked attempts (both from/to set) or one attempt against the app's
// current live state (to omitted). See internal/api/deploy_compare.go's
// own doc comment for the endpoint this loads.
export const Route = createFileRoute('/apps/$name/deploys/compare')({
  validateSearch: validateDeployCompareSearch,
  loaderDeps: ({ search }) => ({ from: search.from, to: search.to }),
  loader: ({ context: { queryClient }, params: { name }, deps: { from, to } }) => {
    if (!from) return undefined
    return queryClient.ensureQueryData(deployCompareQueryOptions(name, from, to))
  },
  component: DeployComparePage,
  pendingComponent: DeployComparePagePending,
})

function DeployComparePage() {
  const { name } = Route.useParams()
  const { from, to } = Route.useSearch()

  if (!from) {
    return (
      <Card>
        <CardContent className="pt-6">
          <EmptyState
            icon={<GitDiffIcon className="size-5" />}
            title="Nothing selected to compare"
            description="Pick two deploys (or one deploy and “compare to current”) from the deploy history list."
            action={
              <Button
                size="sm"
                nativeButton={false}
                render={
                  <Link to="/apps/$name/deploys" params={{ name }} />
                }
              >
                Back to deploy history
              </Button>
            }
          />
        </CardContent>
      </Card>
    )
  }

  return <DeployCompareLoaded appName={name} from={from} to={to} />
}

function DeployCompareLoaded({
  appName,
  from,
  to,
}: {
  appName: string
  from: string
  to?: string
}) {
  const { data: compare } = useDeployCompare(appName, from, to)
  return <DeployCompareView appName={appName} compare={compare} />
}

function DeployComparePagePending() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Comparing deploys</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3" aria-hidden="true">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </CardContent>
    </Card>
  )
}
