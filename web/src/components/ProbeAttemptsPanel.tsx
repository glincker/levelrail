import {
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { ProbeAttempt } from '../types/probeAttempt'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

function outcomeText(a: ProbeAttempt): string {
  if (a.success) return a.status_code ? `ok (${a.status_code})` : 'ok'
  if (a.error) return a.error
  return a.status_code ? `${a.status_code}` : 'failed'
}

// Renders the individual readiness-probe attempts a deploy's cutover
// made (GET .../deploys/{deployId}/probes, internal/probe.WithOnAttempt):
// per-attempt detail (status code, latency) ConditionsPanel's own
// single Reason/Message summary never captures. Rendered only when
// attempts is non-empty (see logs.tsx), the same "nothing to show yet"
// gate ConditionsPanel's caller already applies for an app with no
// readiness probe configured.
export function ProbeAttemptsPanel({ attempts }: { attempts: ProbeAttempt[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Readiness probe attempts</CardTitle>
      </CardHeader>
      <CardContent>
        <ol className="-mx-4 divide-y divide-border">
          {attempts.map((a, i) => (
            <li
              key={a.id}
              className="flex items-start gap-3 px-4 py-2 first:pt-0 last:pb-0"
            >
              {a.success ? (
                <CheckCircleIcon
                  className="mt-0.5 size-4 shrink-0 text-success-foreground"
                  weight="fill"
                  aria-hidden="true"
                />
              ) : (
                <WarningCircleIcon
                  className="mt-0.5 size-4 shrink-0 text-destructive"
                  weight="fill"
                  aria-hidden="true"
                />
              )}
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-foreground">
                  Attempt #{i + 1}{' '}
                  <span className="font-mono text-xs text-muted-foreground">
                    {a.target}
                  </span>
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {outcomeText(a)} · {a.latency_ms}ms
                </p>
              </div>
              <Badge
                variant={a.success ? 'success' : 'destructive'}
                className="mt-0.5 shrink-0 rounded-full"
              >
                {a.success ? 'OK' : 'FAIL'}
              </Badge>
            </li>
          ))}
        </ol>
      </CardContent>
    </Card>
  )
}
