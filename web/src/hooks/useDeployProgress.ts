import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { deployKeys, useDeployStatus } from '../queries/deploys'
import { deployAttemptKeys, useDeployAttempts } from '../queries/deployAttempts'
import { computeDeployStages } from '../lib/deployStages'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'

// How often to re-check while a deploy still looks like it's converging:
// the same cadence deployAttemptsQueryOptions' own RUNNING_ATTEMPT_POLL_
// INTERVAL_MS already uses for "something is actively changing."
const DEPLOY_PROGRESS_POLL_INTERVAL_MS = 3_000

// useDeployProgress combines an app's deploy attempts and reconcile
// conditions (queries/deployAttempts.ts and queries/deploys.ts, kept as
// two separate backend resources per those files' own doc comments) and
// adds the one thing neither query can determine from its own data
// alone: whether to keep polling.
//
// recordInstantDeployAttempt (internal/api/deploys.go) marks a plain
// image-tag redeploy or rollback "succeeded" the instant its desired-
// state write lands, before the application controller's reconcile loop
// has run even once. So for that path (the common one: every redeploy,
// rollback, and promote), attempts[0].status is already 'succeeded' by
// the time this hook's data is fetched, and deployAttemptsQueryOptions'
// own refetchInterval (which only polls while status === 'running')
// never fires - and deployStatusQueryOptions has no refetchInterval at
// all. Without this hook, the roll-out stage and its conditions can sit
// on stale, pre-convergence state indefinitely: an operator has to
// manually reload the page to find out whether a rollback actually
// landed.
//
// computeDeployStages' own rollout-stage fallback (status: 'running'
// when no condition transition has landed at or after the attempt's
// finished_at) is the one signal that actually reflects "still
// converging" for the instant-attempt path, so it drives polling here.
// A still-building attempt (status 'running') is deliberately excluded:
// deployAttemptsQueryOptions' existing interval already covers that
// phase, and computeRolloutStage reports 'pending', not 'running', for
// it, so the two polling mechanisms hand off cleanly rather than racing.
export function useDeployProgress(appName: string): {
  attempts: DeployAttempt[]
  conditions: ReconcileCondition[]
} {
  const queryClient = useQueryClient()
  const attemptsQuery = useDeployAttempts(appName)
  const conditionsQuery = useDeployStatus(appName)
  const latestAttempt = attemptsQuery.data[0]
  const stillConverging =
    latestAttempt !== undefined &&
    computeDeployStages(latestAttempt, conditionsQuery.data, true)[1].status === 'running'

  useEffect(() => {
    if (!stillConverging) return
    const id = setInterval(() => {
      void queryClient.refetchQueries({ queryKey: deployKeys.status(appName) })
      void queryClient.refetchQueries({ queryKey: deployAttemptKeys.list(appName) })
    }, DEPLOY_PROGRESS_POLL_INTERVAL_MS)
    return () => clearInterval(id)
  }, [appName, queryClient, stillConverging])

  return { attempts: attemptsQuery.data, conditions: conditionsQuery.data }
}
