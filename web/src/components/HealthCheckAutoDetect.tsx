import { useState } from 'react'
import {
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  useDiscoverAppHealth,
  type HealthDiscoveryResult,
} from '../queries/healthDiscovery'
import { Button } from '@/components/ui/button'
import { FieldDescription } from '@/components/ui/field'

interface HealthCheckAutoDetectProps {
  appName: string
  // Called with the one path to adopt, exactly like clicking a preset:
  // never invoked automatically, only from an explicit "Use" click.
  onUsePath: (path: string) => void
}

// Active, verified discovery (apps_health_discover.go's own doc
// comment): every path shown here was actually requested against the
// app's running container, never a static guess. "Auto-detect" never
// fills the path field itself, only "Use" does, so nothing is adopted
// without the operator seeing what each path really did.
export function HealthCheckAutoDetect({
  appName,
  onUsePath,
}: HealthCheckAutoDetectProps) {
  const discover = useDiscoverAppHealth()
  const [result, setResult] = useState<HealthDiscoveryResult | null>(null)

  function runDiscovery() {
    setResult(null)
    discover.mutate(appName, { onSuccess: setResult })
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="xs"
          variant="outline"
          disabled={discover.isPending}
          onClick={runDiscovery}
        >
          {discover.isPending ? 'Detecting...' : 'Auto-detect'}
        </Button>
        {discover.isPending ? (
          <FieldDescription>
            Probing the running container, this takes a few seconds.
          </FieldDescription>
        ) : null}
      </div>
      {discover.isError ? (
        <FieldDescription className="text-destructive">
          {discover.error instanceof Error
            ? discover.error.message
            : 'Auto-detect failed.'}
        </FieldDescription>
      ) : null}
      {result ? (
        <HealthDiscoveryResultView result={result} onUsePath={onUsePath} />
      ) : null}
    </div>
  )
}

function HealthDiscoveryResultView({
  result,
  onUsePath,
}: {
  result: HealthDiscoveryResult
  onUsePath: (path: string) => void
}) {
  if (result.found) {
    const found = result.found
    return (
      <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/40 px-2.5 py-1.5 text-sm">
        <CheckCircleIcon
          className="size-4 shrink-0 text-emerald-600"
          aria-hidden="true"
        />
        <span>
          Found a working health check at{' '}
          <span className="font-mono">{found}</span>.
        </span>
        <Button type="button" size="xs" onClick={() => onUsePath(found)}>
          Use it
        </Button>
      </div>
    )
  }

  return (
    <ul className="space-y-1 rounded-md border border-border/60 p-2 text-sm">
      {result.attempts.map((a) => (
        <li key={a.path} className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-1.5">
            {a.success ? (
              <CheckCircleIcon
                className="size-3.5 shrink-0 text-emerald-600"
                aria-hidden="true"
              />
            ) : (
              <WarningCircleIcon
                className="size-3.5 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            )}
            <span className="font-mono">{a.path}</span>
          </span>
          <span className="flex items-center gap-2 text-xs text-muted-foreground">
            <span className="truncate">
              {a.success ? `${a.latency_ms}ms` : a.error}
            </span>
            {a.success ? (
              <Button
                type="button"
                size="xs"
                variant="outline"
                onClick={() => onUsePath(a.path)}
              >
                Use
              </Button>
            ) : null}
          </span>
        </li>
      ))}
    </ul>
  )
}
