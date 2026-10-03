import { createFileRoute } from '@tanstack/react-router'
import { ArrowsLeftRightIcon } from '@phosphor-icons/react/dist/ssr'
import {
  environmentListQueryOptions,
  useEnvironments,
} from '../../../../queries/environments'
import {
  environmentCompareQueryOptions,
  useEnvironmentCompare,
} from '../../../../queries/environmentCompare'
import { EnvironmentEnvCompareView } from '../../../../components/EnvironmentEnvCompareView'
import { Breadcrumbs } from '../../../../components/Breadcrumbs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import { PageHeader } from '@/components/shell/PageHeader'

interface EnvironmentCompareSearch {
  a?: string
  b?: string
}

// A plain function, not a zod schema: matches
// routes/apps/$name/deploys/compare.tsx's own reasoning for the same
// shape of search params (two optional strings, no reason to pull zod
// in just for this).
function validateEnvironmentCompareSearch(
  search: Record<string, unknown>,
): EnvironmentCompareSearch {
  const { a, b } = search
  return {
    a: typeof a === 'string' && a !== '' ? a : undefined,
    b: typeof b === 'string' && b !== '' ? b : undefined,
  }
}

// Reached from ProjectEnvironmentsPanel's "Compare" action (or directly
// via a/b search params): pick two of a project's environments and see
// which env var keys drift between their resolved effective sets. See
// internal/api/environment_compare.go's own doc comment for the
// endpoint this loads and exactly what "effective" means here.
export const Route = createFileRoute('/projects/$id/environments/compare')({
  validateSearch: validateEnvironmentCompareSearch,
  loaderDeps: ({ search }) => ({ a: search.a, b: search.b }),
  loader: ({ context: { queryClient }, params: { id }, deps: { a, b } }) => {
    const tasks: Promise<unknown>[] = [
      queryClient.ensureQueryData(environmentListQueryOptions(id)),
    ]
    if (a && b) {
      tasks.push(
        queryClient.ensureQueryData(environmentCompareQueryOptions(id, a, b)),
      )
    }
    return Promise.all(tasks)
  },
  component: EnvironmentComparePage,
  pendingComponent: EnvironmentComparePagePending,
})

function EnvironmentComparePage() {
  const { id } = Route.useParams()
  const { a, b } = Route.useSearch()
  const navigate = Route.useNavigate()
  const { data: environments } = useEnvironments(id)

  return (
    <div className="space-y-6">
      <PageHeader
        breadcrumb={<Breadcrumbs projectId={id} />}
        title={
          <span className="flex items-center gap-2">
            <ArrowsLeftRightIcon className="size-4 text-muted-foreground" />
            Compare environments
          </span>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle>Pick two environments</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-end gap-3">
          <EnvironmentPicker
            label="A"
            environments={environments}
            value={a}
            onChange={(value) => {
              void navigate({ search: (prev) => ({ ...prev, a: value }) })
            }}
          />
          <EnvironmentPicker
            label="B"
            environments={environments}
            value={b}
            onChange={(value) => {
              void navigate({ search: (prev) => ({ ...prev, b: value }) })
            }}
          />
        </CardContent>
      </Card>

      {!a || !b ? (
        <EmptyState
          icon={<ArrowsLeftRightIcon className="size-5" />}
          title="Pick two environments to compare"
          description="Choose an environment for A and B above to see which env var keys drift between them."
        />
      ) : a === b ? (
        <EmptyState
          icon={<ArrowsLeftRightIcon className="size-5" />}
          title="Pick two different environments"
          description="A and B are the same environment right now; there's nothing to compare."
        />
      ) : (
        <EnvironmentCompareLoaded projectId={id} a={a} b={b} />
      )}
    </div>
  )
}

function EnvironmentPicker({
  label,
  environments,
  value,
  onChange,
}: {
  label: string
  environments: { id: string; name: string }[]
  value: string | undefined
  onChange: (value: string) => void
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </span>
      <Select
        value={value ?? ''}
        onValueChange={(next) => {
          if (next) {
            onChange(next)
          }
        }}
      >
        <SelectTrigger className="w-48">
          <SelectValue placeholder="Select an environment" />
        </SelectTrigger>
        <SelectContent>
          {environments.map((env) => (
            <SelectItem key={env.id} value={env.id}>
              {env.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

function EnvironmentCompareLoaded({
  projectId,
  a,
  b,
}: {
  projectId: string
  a: string
  b: string
}) {
  const { data: compare } = useEnvironmentCompare(projectId, a, b)
  return <EnvironmentEnvCompareView compare={compare} />
}

function EnvironmentComparePagePending() {
  return (
    <div className="space-y-6">
      <div className="h-6 w-48 animate-pulse rounded bg-muted" />
      <Card>
        <CardContent className="space-y-3 pt-6" aria-hidden="true">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-24 w-full" />
        </CardContent>
      </Card>
    </div>
  )
}
