import {
  CheckCircleIcon,
  WarningCircleIcon,
  WarningIcon,
  QuestionIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import type { Icon } from '@phosphor-icons/react'
import type { ConditionStatus, ReconcileCondition } from '../types/deploy'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'

// Reason strings that get a friendlier translation rendered alongside
// their raw Message below, matched on Reason alone rather than on
// resource type: this panel is shared between the app and database
// Overview pages (routes/apps/$name/overview.tsx,
// routes/apps/$name/deploys/index.tsx,
// routes/databases/$name/overview.tsx), so a translation keyed on
// "database-ness" would need a prop this component doesn't otherwise
// need. CredentialsNotConfigured is currently unique to
// internal/reconcile/database/controller.go's credentialsBlockedResult
// (no application controller reason string collides with it), so
// matching on Reason alone is safe today; a future reused Reason string
// would need a resource-type prop added here. A plain function (not a
// lookup table) because Link's `to` prop is a literal route union, not
// `string`: a Record value would widen it and fail typed routing.
function reasonTranslation(reason: string): React.ReactNode {
  if (reason !== 'CredentialsNotConfigured') return null
  return (
    <>
      No secrets master key configured, or this database&rsquo;s own
      credentials failed to generate.{' '}
      <Link to="/settings/general" className="underline underline-offset-2">
        Configure it in Settings &gt; General
      </Link>
      .
    </>
  )
}

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
            const translation = reasonTranslation(c.Reason)
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
                  {translation ? (
                    // Sits above the raw Message below rather than
                    // replacing it: an operator or support engineer
                    // debugging this still benefits from the exact
                    // string the reconciler produced, this is just no
                    // longer the only thing shown. No "warning" tone
                    // exists in badgeVariants/alertVariants, so this
                    // reuses the same amber-precedent classes
                    // CreateDatabaseFields.tsx's own credentials warning
                    // already established for exactly this gap.
                    <div className="mt-1 flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
                      <WarningIcon className="mt-0.5 size-4 shrink-0" />
                      <p className="text-sm">{translation}</p>
                    </div>
                  ) : null}
                  {c.Message ? (
                    <p className="mt-1 text-sm text-muted-foreground">
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
