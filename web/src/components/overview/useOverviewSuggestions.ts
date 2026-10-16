import { summarizeAppStatus } from '../../lib/appStatus'
import type { ReconcileCondition } from '../../types/deploy'
import { ApiError } from '../../lib/apiError'
import { usePendingChanges } from '../../queries/appTimeline'
import { useAppTraffic } from '../../queries/appTraffic'
import { useDiagnosis } from '../../queries/diagnosis'
import { useDomainCheck } from '../../queries/domainCheck'
import { useGitSource } from '../../queries/gitSources'
import { useImageUpdate } from '../../queries/imageUpdate'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import {
  computeSuggestions,
  isAttention,
  isImageDeploy,
  type SuggestionDescriptor,
} from './suggestions'
import type { ResourceReading } from './useResourceReading'
import { useDismissed } from './useDismissed'

export interface OverviewSuggestionData {
  attention: SuggestionDescriptor[]
  setup: SuggestionDescriptor[]
  dismiss: (id: string) => void
}

/** One computation of the suggestions, split into failing signals and setup extras. */
export function useOverviewSuggestions(
  app: AppDetail,
  reading: ResourceReading,
  latest?: DeployAttempt,
  conditions: ReconcileCondition[] = [],
): OverviewSuggestionData {
  const { dismissed, dismiss } = useDismissed(app.name)
  const failed = latest?.status === 'failed'
  const unhealthy = summarizeAppStatus(conditions).label === 'Attention needed'
  const { data: pending } = usePendingChanges(app.name)
  const { data: traffic } = useAppTraffic(app.name)
  const diagnosis = useDiagnosis(app.name, undefined, failed || unhealthy)
  const domain = app.domains?.[0] ?? ''
  const domainCheck = useDomainCheck(app.name, domain)
  const git = useGitSource(app.name)
  const imageUpdate = useImageUpdate(app.image)

  const gitKnown =
    git.isSuccess ||
    (git.isError && git.error instanceof ApiError && git.error.status === 404)

  const all = computeSuggestions({
    app,
    pending,
    traffic,
    memory:
      reading.memNow !== null
        ? { usage: reading.memNow, limit: reading.memLimit }
        : undefined,
    latestAttemptStatus: latest?.status,
    unhealthy,
    topCauseTitle: diagnosis.data?.causes?.[0]?.title,
    domainStatus: domain ? domainCheck.data?.status : undefined,
    gitSourceKnown: gitKnown,
    hasGitSource: Boolean(git.data),
    imageDeployed: isImageDeploy(latest?.source),
    imageUpdate,
    dismissed,
  })
  return {
    attention: all.filter(isAttention),
    setup: all.filter((d) => !isAttention(d)),
    dismiss,
  }
}
