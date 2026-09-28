import { ListChecksIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { usePreflightApp, usePreflightNewApp } from '../queries/preflight'
import type { PreflightRequest } from '../types/preflight'
import { PreflightReportList } from './PreflightReportList'

type RunnerState = ReturnType<typeof usePreflightApp>

function PreflightBody({
  run,
  label,
  pending,
  error,
  report,
}: {
  run: () => void
  label: string
  pending: boolean
  error: Error | null
  report: RunnerState['data']
}) {
  return (
    <div className="space-y-3">
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={pending}
        onClick={run}
      >
        <ListChecksIcon aria-hidden="true" />
        {pending ? 'Checking...' : label}
      </Button>
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error.message}
        </p>
      ) : null}
      {report ? <PreflightReportList report={report} /> : null}
    </div>
  )
}

// "Run preflight" for an existing app: read-only checks of its stored config.
export function AppPreflightCard({ appName }: { appName: string }) {
  const preflight = usePreflightApp(appName)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ListChecksIcon className="size-4" aria-hidden="true" />
          Preflight
        </CardTitle>
      </CardHeader>
      <CardContent>
        <PreflightBody
          run={() => preflight.mutate({})}
          label="Run preflight"
          pending={preflight.isPending}
          error={preflight.error}
          report={preflight.data}
        />
      </CardContent>
    </Card>
  )
}

// Mounted in the create flow: checks the app being described before it exists.
export function NewAppPreflight({ request }: { request: PreflightRequest }) {
  const preflight = usePreflightNewApp()
  return (
    <PreflightBody
      run={() => preflight.mutate(request)}
      label="Check before creating"
      pending={preflight.isPending}
      error={preflight.error}
      report={preflight.data}
    />
  )
}
