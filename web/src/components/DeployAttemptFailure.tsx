import { CircleAlertGlassIcon } from '@thesvg/ui-react'
import type { DeployAttempt } from '../types/deployAttempt'
import { DeployFailureActions } from './DeployFailureActions'
import { DeployFailureDetails } from './DeployFailureDetails'
import { DeployFromCacheButton } from './DeployFromCacheButton'

export function DeployAttemptFailure({
  appName,
  attempt,
}: {
  appName: string
  attempt: DeployAttempt
}) {
  if (!attempt.failure && !attempt.error) return null
  return (
    <div className="mt-2 flex items-start gap-2 rounded-md border border-tone-danger-border bg-tone-danger-soft px-3 py-2 text-sm">
      <CircleAlertGlassIcon
        className="mt-0.5 size-5 shrink-0 text-tone-danger"
        aria-hidden="true"
      />
      <div className="min-w-0">
        <p className="line-clamp-2 font-medium break-words text-tone-danger">
          {attempt.failure?.cause ?? attempt.error}
        </p>
        {attempt.failure?.suggested_fix ? (
          <p className="mt-0.5 line-clamp-2 text-xs break-words text-muted-foreground">
            {attempt.failure.suggested_fix}
          </p>
        ) : null}
        <DeployFailureDetails failure={attempt.failure} />
        {attempt.failure ? (
          <DeployFailureActions
            failure={attempt.failure}
            appName={appName}
            image={attempt.image}
          />
        ) : null}
        {attempt.failure?.code === 'git_host_unreachable' ? (
          <DeployFromCacheButton appName={appName} />
        ) : null}
      </div>
    </div>
  )
}
