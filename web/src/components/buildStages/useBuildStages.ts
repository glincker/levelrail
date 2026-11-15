import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { useNowTick } from '../../hooks/useNowTick'
import type { DeployStepEvent } from '../../hooks/useDeployStepStream'
import { buildStagesQueryOptions } from '../../queries/buildStages'
import { hasRunningStage, isBuildStepKey } from '../../lib/buildStages'

/** Polled stage data shared by every section: same key, one request. */
export function useBuildStages(appName: string, deployId: string) {
  const query = useQuery(buildStagesQueryOptions(appName, deployId))
  const running = query.data ? hasRunningStage(query.data) : false
  const nowMs = useNowTick(running)
  return { query, data: query.data, nowMs }
}

/** A build_stage step event means the table changed: refetch now. */
export function useRefetchOnBuildSteps(
  appName: string,
  deployId: string,
  steps: DeployStepEvent[],
): void {
  const { refetch } = useQuery(buildStagesQueryOptions(appName, deployId))
  const signature = useMemo(
    () =>
      steps
        .filter((s) => isBuildStepKey(s.key))
        .map((s) => `${s.key ?? ''}:${s.status}`)
        .join('|'),
    [steps],
  )
  useEffect(() => {
    if (signature) void refetch()
  }, [signature, refetch])
}
