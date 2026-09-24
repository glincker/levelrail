import { useId, useState } from 'react'
import {
  ArrowCounterClockwiseIcon,
  ArrowsClockwiseIcon,
  LightbulbIcon,
  TerminalIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { LogLine } from '../hooks/useLogStream'
import type { ReconcileCondition } from '../types/deploy'
import type { DeployAttempt } from '../types/deployAttempt'
import type { DeployStage } from '../lib/deployStages'
import {
  deployStageAnchorId,
  findLastGoodAttempt,
  summarizeDeployFailure,
} from '../lib/deployFailureSummary'
import { useRedeployApp } from '../hooks/useRedeployApp'
import { useTriggerDeploy } from '../queries/deploys'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'

const MESSAGE_PREVIEW_CHARS = 240

// "What went wrong" card shown above a failed attempt's details. Cause
// and fix come from heuristic log rules, so they are labelled "likely".
export function DeployFailureSummaryCard({
  appName,
  attempt,
  attempts,
  stages,
  conditions,
  lines,
}: {
  appName: string
  attempt: DeployAttempt
  attempts: DeployAttempt[]
  stages: DeployStage[]
  conditions: ReconcileCondition[]
  lines: LogLine[]
}) {
  const headingId = useId()
  const messageId = useId()
  const [expanded, setExpanded] = useState(false)
  const retry = useRedeployApp(appName, attempt.image)
  const rollback = useTriggerDeploy(appName)

  const summary = summarizeDeployFailure({
    stages,
    conditions,
    lines,
    attempt,
  })
  if (!summary) {
    return null
  }
  const lastGood = findLastGoodAttempt(attempts, attempt)
  const long = summary.message.length > MESSAGE_PREVIEW_CHARS
  const shown =
    long && !expanded
      ? `${summary.message.slice(0, MESSAGE_PREVIEW_CHARS)}...`
      : summary.message

  const viewLogs = () => {
    const el = document.getElementById(deployStageAnchorId(summary.stageKey))
    el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    el?.focus({ preventScroll: true })
  }

  const rollBack = () => {
    if (!lastGood) return
    rollback.mutate(
      { image: lastGood.image },
      {
        onSuccess: () => {
          toast.add({
            title: 'Rollback triggered.',
            description: `Redeploying ${lastGood.image}.`,
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({ title: error.message, type: 'error' })
        },
      },
    )
  }

  return (
    <section aria-labelledby={headingId}>
      <Card className="border-destructive/40">
        <CardContent className="space-y-3">
          <h2
            id={headingId}
            className="flex items-center gap-2 text-sm font-semibold text-foreground"
          >
            <WarningCircleIcon
              className="size-4 text-destructive"
              aria-hidden="true"
            />
            What went wrong
          </h2>
          <dl className="space-y-3 text-sm">
            <div>
              <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                Failing stage
              </dt>
              <dd className="mt-1 text-foreground">{summary.stageLabel}</dd>
            </div>
            <div>
              <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                Error
              </dt>
              <dd className="mt-1">
                <p
                  id={messageId}
                  className="break-words whitespace-pre-wrap rounded bg-muted px-2 py-1.5 font-mono text-xs text-foreground"
                >
                  {shown}
                </p>
                {long ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    className="mt-1"
                    aria-expanded={expanded}
                    aria-controls={messageId}
                    onClick={() => setExpanded((v) => !v)}
                  >
                    {expanded ? 'Show less' : 'Show more'}
                  </Button>
                ) : null}
              </dd>
            </div>
            {summary.cause ? (
              <div>
                <dt className="flex items-center gap-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  <LightbulbIcon className="size-3.5" aria-hidden="true" />
                  Likely cause and fix
                </dt>
                <dd className="mt-1 text-foreground">
                  <p className="font-medium">{summary.cause.title}</p>
                  <p className="text-muted-foreground">{summary.cause.hint}</p>
                  {summary.evidence ? (
                    <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                      {summary.evidence}
                    </p>
                  ) : null}
                </dd>
              </div>
            ) : null}
          </dl>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={viewLogs}
            >
              <TerminalIcon aria-hidden="true" />
              View full logs
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={retry.isPending}
              onClick={retry.redeploy}
            >
              <ArrowsClockwiseIcon aria-hidden="true" />
              Retry deploy
            </Button>
            {lastGood ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={rollback.isPending}
                onClick={rollBack}
              >
                <ArrowCounterClockwiseIcon aria-hidden="true" />
                Roll back to last good
              </Button>
            ) : null}
          </div>
        </CardContent>
      </Card>
    </section>
  )
}
