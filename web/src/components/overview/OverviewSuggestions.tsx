import { SuggestionList } from '@/components/kit'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ApiError } from '../../lib/apiError'
import { usePendingChanges } from '../../queries/appTimeline'
import { useAppTraffic } from '../../queries/appTraffic'
import { useDiagnosis } from '../../queries/diagnosis'
import { useDomainCheck } from '../../queries/domainCheck'
import { useGitSource } from '../../queries/gitSources'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import { DiagnosisFixes } from '../DiagnosisFixes'
import { computeSuggestions } from './suggestions'
import type { ResourceReading } from './useResourceReading'
import { toSuggestionItems, useSuggestionActions } from './useSuggestionActions'
import { useDismissed } from './useDismissed'

export function OverviewSuggestions({
  app,
  reading,
  latest,
}: {
  app: AppDetail
  reading: ResourceReading
  latest?: DeployAttempt
}) {
  const { dismissed, dismiss } = useDismissed(app.name)
  const failed = latest?.status === 'failed'
  const { data: pending } = usePendingChanges(app.name)
  const { data: traffic } = useAppTraffic(app.name)
  const diagnosis = useDiagnosis(app.name, undefined, failed)
  const domain = app.domains?.[0] ?? ''
  const domainCheck = useDomainCheck(app.name, domain)
  const git = useGitSource(app.name)
  const actions = useSuggestionActions(app, latest)

  const gitKnown =
    git.isSuccess ||
    (git.isError && git.error instanceof ApiError && git.error.status === 404)

  const descriptors = computeSuggestions({
    app,
    pending,
    traffic,
    memory:
      reading.memNow !== null
        ? { usage: reading.memNow, limit: reading.memLimit }
        : undefined,
    latestAttemptStatus: latest?.status,
    topCauseTitle: diagnosis.data?.causes?.[0]?.title,
    domainStatus: domain ? domainCheck.data?.status : undefined,
    gitSourceKnown: gitKnown,
    hasGitSource: Boolean(git.data),
    dismissed,
  })

  return (
    <>
      <SuggestionList
        items={toSuggestionItems(descriptors, actions, dismiss)}
      />
      <Dialog open={actions.fixOpen} onOpenChange={actions.setFixOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Fix the failed deploy</DialogTitle>
          </DialogHeader>
          <DiagnosisFixes appName={app.name} deployId={latest?.id} />
        </DialogContent>
      </Dialog>
    </>
  )
}
