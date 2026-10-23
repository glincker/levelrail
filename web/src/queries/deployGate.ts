import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  DeployGate,
  DeploySafety,
  DeploySafetyOverrides,
  RollbackTarget,
} from '../types/deployGate'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Polls a release while its gate or post-deploy watch can still change.
const GATE_POLL_MS = 3000

export const deployGateKeys = {
  gate: (appName: string, deployId: string) =>
    [...appKeys.detail(appName), 'deploy-gate', deployId] as const,
  safety: (appName: string) =>
    [...appKeys.detail(appName), 'deploy-safety'] as const,
  targets: (appName: string) =>
    [...appKeys.detail(appName), 'rollback-targets'] as const,
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function appPath(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}`
}

export function deployGateQueryOptions(appName: string, deployId: string) {
  return queryOptions({
    queryKey: deployGateKeys.gate(appName, deployId),
    queryFn: () =>
      getJson<DeployGate>(
        `${appPath(appName)}/deploys/${encodeURIComponent(deployId)}/gate`,
        'fetch deploy gate',
      ),
    refetchInterval: (query) =>
      query.state.data?.terminal ? false : GATE_POLL_MS,
  })
}

export function useDeployGate(appName: string, deployId: string) {
  return useQuery(deployGateQueryOptions(appName, deployId))
}

export function deploySafetyQueryOptions(appName: string) {
  return queryOptions({
    queryKey: deployGateKeys.safety(appName),
    queryFn: () =>
      getJson<DeploySafety>(
        `${appPath(appName)}/deploy-safety`,
        'fetch deploy safety settings',
      ),
  })
}

export function useDeploySafety(appName: string) {
  return useQuery(deploySafetyQueryOptions(appName))
}

export function useSetDeploySafety(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<DeploySafety, ApiError, DeploySafetyOverrides>({
    mutationFn: async (overrides) => {
      const res = await fetch(`${appPath(appName)}/deploy-safety`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(overrides),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `save deploy safety settings failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as DeploySafety
    },
    onSuccess: (result) => {
      queryClient.setQueryData(deployGateKeys.safety(appName), result)
    },
  })
}

export function rollbackTargetsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: deployGateKeys.targets(appName),
    queryFn: async () =>
      (
        await getJson<{ targets: RollbackTarget[] }>(
          `${appPath(appName)}/rollback-targets`,
          'fetch rollback targets',
        )
      ).targets,
  })
}

export function useRollbackTargets(appName: string) {
  return useQuery(rollbackTargetsQueryOptions(appName))
}

export function useStopWatch(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<unknown, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(`${appPath(appName)}/watch/stop`, {
        method: 'POST',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `stop watch failed: ${res.status}`),
        )
      }
      return (await res.json()) as unknown
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) }),
  })
}
