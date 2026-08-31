import { useState } from 'react'
import { CaretDownIcon, WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsiblePanel,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import { useDiagnosis } from '../queries/diagnosis'
import { useAlertRules } from '../queries/alerts'
import type { ReconcileCondition } from '../types/deploy'
import type { DeployAttemptStatus } from '../types/deployAttempt'
import type { DiagnosisConfidence } from '../types/diagnosis'
import type { VariantProps } from 'class-variance-authority'

const CONFIDENCE_LABEL: Record<DiagnosisConfidence, string> = {
  high: 'High confidence',
  medium: 'Possible cause',
  none: 'No known pattern matched',
}

const CONFIDENCE_BADGE_VARIANT: Record<
  DiagnosisConfidence,
  VariantProps<typeof badgeVariants>['variant']
> = {
  high: 'destructive',
  medium: 'warning',
  none: 'muted',
}

// Renders nothing for a healthy app: this panel only exists to explain
// a failure, so it stays out of the way otherwise. `open` starts true
// so an operator who just watched a deploy fail sees the explanation
// immediately, not behind an extra click; the collapsible chrome is
// still there for dismissing it without losing the rest of the page.
export function DiagnosisPanel({
  appName,
  conditions,
  latestAttemptStatus,
}: {
  appName: string
  conditions: ReconcileCondition[]
  latestAttemptStatus: DeployAttemptStatus | undefined
}) {
  const [open, setOpen] = useState(true)

  // useAlertRules already exists for the Alerts tab; reused here purely
  // as a read, no new endpoint, so this panel can also auto-show for a
  // crashlooping app whose reconcile conditions still read healthy
  // (the exact gap crashloop detection exists to catch). A control
  // plane with no alerting configured (501) just means this signal is
  // unavailable, not an error worth surfacing here.
  const { data: alertRules } = useAlertRules(appName)
  const crashlooping =
    alertRules?.some((r) => r.kind === 'crashloop' && r.firing) ?? false
  const attemptFailed = latestAttemptStatus === 'failed'
  const conditionFailing = conditions.some((c) => c.Status === 'False')
  const shouldShow = attemptFailed || conditionFailing || crashlooping

  const { data, isLoading, isError } = useDiagnosis(
    appName,
    undefined,
    shouldShow,
  )

  if (!shouldShow) {
    return null
  }

  return (
    <Card>
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger>
          <CardHeader className="flex-row items-center justify-between gap-3">
            <CardTitle className="flex items-center gap-2">
              <WarningCircleIcon className="size-4 text-destructive" />
              What happened?
            </CardTitle>
            <CaretDownIcon
              className={cn(
                'size-4 shrink-0 text-muted-foreground transition-transform',
                open && 'rotate-180',
              )}
              aria-hidden="true"
            />
          </CardHeader>
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <CardContent>
            {isLoading ? (
              <p className="text-sm text-muted-foreground">Diagnosing…</p>
            ) : isError ? (
              <p className="text-sm text-muted-foreground">
                Could not load a diagnosis right now.
              </p>
            ) : data ? (
              <div className="space-y-3">
                <Badge variant={CONFIDENCE_BADGE_VARIANT[data.confidence]}>
                  {CONFIDENCE_LABEL[data.confidence]}
                </Badge>
                <p className="text-sm text-foreground">{data.explanation}</p>
                <div className="rounded-lg border border-border bg-muted/40 p-3">
                  <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    Suggested next step
                  </p>
                  <p className="mt-1 text-sm text-foreground">
                    {data.suggestion}
                  </p>
                </div>
                {data.matched_signals.length > 0 ? (
                  <div>
                    <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                      Matched signals
                    </p>
                    <ul className="mt-1 space-y-1">
                      {data.matched_signals.map((s, i) => (
                        <li
                          key={`${s.source}-${i}`}
                          className="font-mono text-xs text-muted-foreground"
                        >
                          [{s.source}] {s.excerpt}
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : null}
              </div>
            ) : null}
          </CardContent>
        </CollapsiblePanel>
      </Collapsible>
    </Card>
  )
}
