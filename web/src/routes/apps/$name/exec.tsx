import { createFileRoute } from '@tanstack/react-router'
import { AppTerminal } from '../../../components/AppTerminal'
import { ExecPanel } from '../../../components/ExecPanel'
import { ExecAccessCard } from '../../../components/ExecAccessCard'
import {
  execAccessQueryOptions,
  useExecAccess,
} from '../../../queries/execAccess'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// Two ways into the same container, in the order an operator reaches for
// them: an interactive shell (GET /api/v1/apps/{name}/terminal) for
// poking around, and the one-off command runner (POST
// /api/v1/apps/{name}/exec) for a single scripted check whose output you
// want to read rather than drive. Neither needs the app's full detail
// record, only its name. ExecAccessCard is the per-app opt-out both of
// those routes check server-side (internal/api/exec.go's
// requireExecAccess): loaded here, once, and passed down as a plain
// prop so AppTerminal/ExecPanel stay simple rendering, not another
// suspense query each.
export const Route = createFileRoute('/apps/$name/exec')({
  loader: async ({ context: { queryClient }, params: { name } }) => {
    await queryClient.ensureQueryData(execAccessQueryOptions(name))
  },
  component: ExecSection,
  pendingComponent: ExecSectionPending,
})

function ExecSection() {
  const { name } = Route.useParams()
  const execAccess = useExecAccess(name)
  const enabled = execAccess.data.enabled

  return (
    <div className="space-y-4">
      <ExecAccessCard appName={name} />
      <AppTerminal name={name} execEnabled={enabled} />
      <ExecPanel name={name} execEnabled={enabled} />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, one skeleton per
// card so the page doesn't jump when the real ones swap in.
function ExecSectionPending() {
  return (
    <div className="space-y-4">
      {[0, 1, 2].map((i) => (
        <Card key={i}>
          <CardHeader>
            <CardTitle>
              <Skeleton className="h-5 w-40" />
            </CardTitle>
          </CardHeader>
          <CardContent aria-hidden="true">
            <Skeleton className="h-16 w-full" />
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
