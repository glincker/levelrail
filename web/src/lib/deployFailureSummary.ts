import type { LogLine } from '../hooks/useLogStream'
import type { ReconcileCondition } from '../types/deploy'
import type { DeployAttempt } from '../types/deployAttempt'
import type { DeployStage } from './deployStages'
import { stripAnsiCodes } from './ansi'
import { matchHintForText } from './buildLogHints'

export interface DeployFailureSummary {
  stageKey: DeployStage['key']
  stageLabel: string
  message: string
  cause?: { id: string; title: string; hint: string }
  evidence?: string
}

const MAX_SCANNED_LINES = 500
const NO_MESSAGE = 'No error message was recorded for this failure.'

// summarizeDeployFailure explains a failed attempt: the failing stage,
// its error text and, when a rule matches, the likely cause. The error
// and failing conditions are tried first, then log lines newest-first
// (the last error line is usually the real one).
export function summarizeDeployFailure(input: {
  stages: DeployStage[]
  conditions: ReconcileCondition[]
  lines: LogLine[]
  attempt: DeployAttempt
}): DeployFailureSummary | null {
  const stage = input.stages.find((s) => s.status === 'failed')
  if (!stage) {
    return null
  }
  const message = (stage.detail || input.attempt.error || '').trim()
  const summary: DeployFailureSummary = {
    stageKey: stage.key,
    stageLabel: stage.label,
    message: message || NO_MESSAGE,
  }

  const failing = input.conditions
    .filter((c) => c.Status === 'False')
    .map((c) => `${c.Reason} ${c.Message}`)
  for (const text of [message, input.attempt.error ?? '', ...failing]) {
    const cause = text ? matchHintForText(text) : undefined
    if (cause) {
      return { ...summary, cause }
    }
  }

  const start = Math.max(0, input.lines.length - MAX_SCANNED_LINES)
  for (let i = input.lines.length - 1; i >= start; i--) {
    const text = stripAnsiCodes(input.lines[i]?.line ?? '')
    const cause = matchHintForText(text)
    if (cause) {
      return { ...summary, cause, evidence: text.trim() }
    }
  }
  return summary
}

// findLastGoodAttempt returns the newest succeeded attempt that started
// before the given one, or undefined when there is none.
export function findLastGoodAttempt(
  attempts: DeployAttempt[],
  attempt: DeployAttempt,
): DeployAttempt | undefined {
  const failedAt = new Date(attempt.started_at).getTime()
  return attempts
    .filter(
      (a) =>
        a.id !== attempt.id &&
        a.status === 'succeeded' &&
        new Date(a.started_at).getTime() < failedAt,
    )
    .sort(
      (a, b) =>
        new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
    )[0]
}

export function deployStageAnchorId(key: DeployStage['key']): string {
  return `deploy-stage-${key}`
}
