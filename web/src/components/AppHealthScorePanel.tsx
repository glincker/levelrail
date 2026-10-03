import {
  CheckCircleIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import { Skeleton } from '@/components/ui/skeleton'
import { useAppHealthScore } from '../queries/healthScore'
import type { HealthScoreStatus } from '../types/healthScore'

// Per-app "is this production-ready" summary, synthesizing signals this
// dashboard otherwise shows only one at a time on separate pages (deploy
// history, alerts, domains/TLS, secrets, volume backups): see
// GET /api/v1/apps/{name}/health-score's own doc comment
// (internal/api/app_health_score.go) for exactly which real store reads
// back each category. Deliberately not a numeric score: a vague
// percentage hides which specific thing needs attention, a one-line
// reason per category does not.

const STATUS_LABEL: Record<HealthScoreStatus, string> = {
  pass: 'Pass',
  warn: 'Warn',
  fail: 'Fail',
}

const STATUS_BADGE_VARIANT: Record<
  HealthScoreStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
}

const STATUS_ICON: Record<HealthScoreStatus, Icon> = {
  pass: CheckCircleIcon,
  warn: WarningIcon,
  fail: WarningCircleIcon,
}

const STATUS_ICON_CLASS: Record<HealthScoreStatus, string> = {
  pass: 'text-green-600 dark:text-green-400',
  warn: 'text-amber-600 dark:text-amber-400',
  fail: 'text-destructive',
}

export function AppHealthScorePanel({ appName }: { appName: string }) {
  const { data: score, isLoading, error } = useAppHealthScore(appName)

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3">
        <CardTitle className="text-sm font-semibold">
          Health & readiness
        </CardTitle>
        {score ? (
          <Badge variant={STATUS_BADGE_VARIANT[score.status]} className="gap-1">
            {(() => {
              const Icon = STATUS_ICON[score.status]
              return <Icon aria-hidden="true" />
            })()}
            {STATUS_LABEL[score.status]}
          </Badge>
        ) : null}
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2" aria-hidden="true">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : (
          <ul className="divide-y divide-border">
            {score?.categories.map((category) => {
              const Icon = STATUS_ICON[category.status]
              return (
                <li
                  key={category.key}
                  className="flex items-start gap-3 py-2.5 first:pt-0 last:pb-0"
                >
                  <Icon
                    className={`mt-0.5 size-4 shrink-0 ${STATUS_ICON_CLASS[category.status]}`}
                    aria-hidden="true"
                  />
                  <div className="flex-1 space-y-0.5">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-foreground">
                        {category.label}
                      </span>
                      <Badge variant={STATUS_BADGE_VARIANT[category.status]}>
                        {STATUS_LABEL[category.status]}
                      </Badge>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {category.reason}
                    </p>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
