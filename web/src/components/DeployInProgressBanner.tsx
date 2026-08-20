import { Link } from '@tanstack/react-router'
import { RocketIcon } from '@phosphor-icons/react/dist/ssr'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import { computeDeployStages } from '../lib/deployStages'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

// Surfaced at the top of the app overview page whenever the app's latest
// deploy attempt is still running, so an operator lands directly on
// "something is happening right now" instead of having to open Deploy
// history to notice. Links into the same live stage timeline / log
// viewer route DeployAttemptsList's own "Logs" link already uses.
export function DeployInProgressBanner({
  appName,
  attempt,
  conditions,
}: {
  appName: string
  attempt: DeployAttempt
  conditions: ReconcileCondition[]
}) {
  const stages = computeDeployStages(attempt, conditions, true)
  const currentStage = stages.find((s) => s.status === 'running') ?? stages[0]

  return (
    <Card className="ring-primary/20">
      <CardContent className="flex flex-wrap items-center justify-between gap-3 py-4">
        <div className="flex min-w-0 items-center gap-3">
          <RocketIcon className="size-5 shrink-0 animate-pulse text-primary" aria-hidden="true" />
          <div className="min-w-0">
            <p className="flex items-center gap-2 text-sm font-medium text-foreground">
              Deploy in progress
              <Badge variant="muted">{currentStage.label}</Badge>
            </p>
            <p className="truncate text-xs text-muted-foreground">
              {attempt.image}
            </p>
          </div>
        </div>
        <Button
          size="sm"
          nativeButton={false}
          render={
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: appName, deployId: attempt.id }}
            />
          }
        >
          Watch live
        </Button>
      </CardContent>
    </Card>
  )
}
