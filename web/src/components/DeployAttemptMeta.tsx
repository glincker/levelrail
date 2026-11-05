import { CommitGraphGlassIcon, GitBranchGlassIcon } from '@thesvg/ui-react'
import type { DeployAttempt } from '../types/deployAttempt'
import { formatDeployDuration } from '../lib/deployDuration'
import { DEPLOY_ATTEMPT_SOURCE_LABEL } from '../lib/deployAttemptPresentation'
import { RelativeTime } from './kit'
import { RolloutChip } from './DeployDigestChips'

function durationSuffix(attempt: DeployAttempt): string {
  if (attempt.finished_at) {
    const ms =
      new Date(attempt.finished_at).getTime() -
      new Date(attempt.started_at).getTime()
    if (ms < 1000) return ''
  }
  return ` · ${formatDeployDuration(attempt.started_at, attempt.finished_at)}`
}

export function DeployAttemptMeta({ attempt }: { attempt: DeployAttempt }) {
  return (
    <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {attempt.branch ? (
        <span className="inline-flex items-center gap-1">
          <GitBranchGlassIcon className="size-5" aria-hidden="true" />
          <span className="font-mono">{attempt.branch}</span>
        </span>
      ) : null}
      {attempt.commit_sha ? (
        <span className="inline-flex items-center gap-1 font-mono">
          <CommitGraphGlassIcon className="size-5" aria-hidden="true" />
          {attempt.commit_sha.slice(0, 7)}
        </span>
      ) : null}
      {attempt.source ? (
        <span>{DEPLOY_ATTEMPT_SOURCE_LABEL[attempt.source]}</span>
      ) : null}
      {attempt.author ? <span>{attempt.author}</span> : null}
      <span title={new Date(attempt.started_at).toLocaleString()}>
        <RelativeTime at={attempt.started_at} live />
        {durationSuffix(attempt)}
      </span>
      {attempt.rollout_state === 'mismatch' ? (
        <RolloutChip attempt={attempt} />
      ) : null}
    </p>
  )
}
