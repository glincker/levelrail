import {
  CheckCircle2Icon,
  CircleAlertIcon,
  HelpCircleIcon,
  type LucideIcon,
} from 'lucide-react'
import type { ConditionStatus, ReconcileCondition } from '../types/deploy'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'

// Sourced from the shared success/destructive/muted variants in
// badgeVariants (components/ui/badge.tsx) rather than a locally hand-rolled
// class string.
const STATUS_BADGE_VARIANT: Record<
  ConditionStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  True: 'success',
  False: 'destructive',
  Unknown: 'muted',
}

const STATUS_ICON: Record<ConditionStatus, LucideIcon> = {
  True: CheckCircle2Icon,
  False: CircleAlertIcon,
  Unknown: HelpCircleIcon,
}

// Renders the app's *current* reconcile status, not a deploy history
// log. GET /api/v1/apps/{name}/deploys (internal/api/deploys.go's
// handleDeployHistory) returns the latest condition per (controller,
// condition type) pair, since internal/store.UpsertConditions only ever
// keeps the newest row for each type; there is no attempt-by-attempt
// log to render. The heading and copy below are deliberately worded as
// "status", not "history", to match what the endpoint actually returns.
export function ConditionsPanel({
  conditions,
}: {
  conditions: ReconcileCondition[]
}) {
  if (conditions.length === 0) {
    return (
      <Card>
        <CardContent className="text-sm text-muted-foreground">
          No reconcile status recorded yet.
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Reconcile status</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="-mx-4 divide-y divide-border">
          {conditions.map((c) => {
            const Icon = STATUS_ICON[c.Status]
            return (
              <li
                key={c.Type}
                className="flex items-start gap-3 px-4 py-3 first:pt-0 last:pb-0"
              >
                <Badge
                  variant={STATUS_BADGE_VARIANT[c.Status]}
                  className="mt-0.5 rounded-full"
                >
                  <Icon className="size-3" />
                  {c.Status}
                </Badge>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-foreground">
                    {c.Type}: {c.Reason}
                  </p>
                  {c.Message ? (
                    <p className="mt-0.5 text-sm text-muted-foreground">
                      {c.Message}
                    </p>
                  ) : null}
                  <p className="mt-1 text-xs text-muted-foreground/70">
                    Updated {new Date(c.LastTransitionTime).toLocaleString()}
                  </p>
                </div>
              </li>
            )
          })}
        </ul>
      </CardContent>
    </Card>
  )
}
