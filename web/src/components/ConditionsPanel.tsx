import {
  CheckCircleIcon,
  WarningCircleIcon,
  QuestionIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { ConditionStatus, ReconcileCondition } from '../types/deploy'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const STATUS_BADGE_CLASS: Record<ConditionStatus, string> = {
  True: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  False: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  Unknown: 'bg-muted text-muted-foreground',
}

const STATUS_ICON: Record<ConditionStatus, Icon> = {
  True: CheckCircleIcon,
  False: WarningCircleIcon,
  Unknown: QuestionIcon,
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
                <span
                  className={`mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_BADGE_CLASS[c.Status]}`}
                >
                  <Icon className="size-3" />
                  {c.Status}
                </span>
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
