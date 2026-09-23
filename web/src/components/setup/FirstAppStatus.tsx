import { Link } from '@tanstack/react-router'
import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  CircleNotchIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import type { AppListEntry } from '../../types/appDetail'
import { useAppNetwork } from '../../queries/appNetwork'
import { useDiagnosis } from '../../queries/diagnosis'
import { liveUrl } from '../../lib/setupWizard'
import type { AppPhase } from '../../lib/setupWizard'

/** FirstAppStatus renders the tracked app's deploy progress, its live URL, or its diagnosis. */
export function FirstAppStatus({
  app,
  phase,
}: {
  app: AppListEntry
  phase: AppPhase
}) {
  const { data: network } = useAppNetwork(app.name)
  const url = liveUrl(app, network, window.location.hostname)
  const failed = phase === 'failed'
  const { data: diagnosis, isLoading: diagnosing } = useDiagnosis(
    app.name,
    undefined,
    failed,
  )

  const appLinks = (
    <div className="flex flex-wrap gap-2">
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/apps/$name" params={{ name: app.name }} />}
      >
        Open app
      </Button>
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/apps/$name/logs" params={{ name: app.name }} />}
      >
        View logs
      </Button>
    </div>
  )

  if (phase === 'live') {
    return (
      <div className="space-y-3 rounded-lg border border-green-600/30 bg-green-50 p-4 dark:bg-green-950/40">
        <p className="flex items-center gap-2 text-base font-semibold text-foreground">
          <CheckCircleIcon className="size-5 text-green-600 dark:text-green-400" />
          It&apos;s live
        </p>
        <p className="text-sm text-muted-foreground">
          {app.name} passed its health check and is serving traffic.
        </p>
        {url ? (
          <Button
            size="sm"
            render={<a href={url} target="_blank" rel="noreferrer" />}
          >
            {url}
            <ArrowSquareOutIcon />
          </Button>
        ) : (
          <p className="text-xs text-muted-foreground">
            It has no public address yet. Add a domain on the app&apos;s Domains
            tab to reach it from the internet.
          </p>
        )}
        {appLinks}
      </div>
    )
  }

  if (failed) {
    return (
      <div className="space-y-3 rounded-lg border border-destructive/40 p-4">
        <p className="flex items-center gap-2 text-sm font-semibold text-foreground">
          <WarningCircleIcon className="size-5 text-destructive" />
          {app.name} is not healthy
        </p>
        {diagnosing ? (
          <p className="text-xs text-muted-foreground">Diagnosing...</p>
        ) : null}
        {diagnosis ? (
          <div className="space-y-1.5 text-sm">
            <p className="text-foreground">{diagnosis.explanation}</p>
            {diagnosis.suggestion ? (
              <p className="text-muted-foreground">{diagnosis.suggestion}</p>
            ) : null}
            {diagnosis.matched_signals.slice(0, 3).map((s) => (
              <pre
                key={`${s.source}:${s.excerpt}`}
                className="overflow-x-auto rounded-md bg-muted/50 p-2 text-xs whitespace-pre-wrap"
              >
                {s.excerpt}
              </pre>
            ))}
          </div>
        ) : null}
        {appLinks}
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-lg border border-border p-4">
      <p className="flex items-center gap-2 text-sm font-semibold text-foreground">
        <CircleNotchIcon className="size-5 animate-spin text-primary motion-reduce:animate-none" />
        Deploying {app.name}
      </p>
      <p className="text-sm text-muted-foreground">
        {app.status.label}. Pulling the image, starting the container, and
        waiting for the health check to pass.
      </p>
      {phase === 'slow' ? (
        <p className="text-xs text-muted-foreground">
          This is taking longer than usual. Large images take a while on a slow
          connection; the logs show what is happening.
        </p>
      ) : null}
      {appLinks}
    </div>
  )
}
