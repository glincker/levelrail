import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { EnvironmentKind, GlobalEnvironment } from '../types/environment'
import type { DeployApprovalResource } from '../types/deployApproval'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { databaseKeys } from './databases'
import { deployApprovalKeys } from './deployApprovals'

export const globalEnvironmentKeys = {
  all: ['global-environments'] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
    )
  }
  return (await res.json().catch(() => ({}))) as T
}

function json(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function globalEnvironmentsQueryOptions() {
  return queryOptions({
    queryKey: globalEnvironmentKeys.all,
    queryFn: () =>
      request<GlobalEnvironment[]>(
        '/api/v1/environments',
        undefined,
        'list environments',
      ),
  })
}

// Plain query so a shell component can mount it behind the feature gate.
export function useGlobalEnvironments(enabled = true) {
  return useQuery({ ...globalEnvironmentsQueryOptions(), enabled })
}

export interface EnvironmentInput {
  name: string
  kind: EnvironmentKind
  protected: boolean
}

export function useSaveEnvironment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: EnvironmentInput & { id?: string }) =>
      id
        ? request<GlobalEnvironment>(
            `/api/v1/environments/${encodeURIComponent(id)}`,
            json('PATCH', input),
            'update environment',
          )
        : request<GlobalEnvironment>(
            '/api/v1/environments',
            json('POST', input),
            'create environment',
          ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: globalEnvironmentKeys.all }),
  })
}

export function useSetEnvironmentProtectedGlobal() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      protectedFlag,
    }: {
      id: string
      protectedFlag: boolean
    }) =>
      request<GlobalEnvironment>(
        `/api/v1/environments/${encodeURIComponent(id)}`,
        json('PATCH', { protected: protectedFlag }),
        'update environment',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: globalEnvironmentKeys.all }),
  })
}

export function useDeleteGlobalEnvironment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, moveTo }: { id: string; moveTo?: string }) =>
      request<unknown>(
        `/api/v1/environments/${encodeURIComponent(id)}${moveTo ? `?move_to=${encodeURIComponent(moveTo)}` : ''}`,
        { method: 'DELETE' },
        'delete environment',
      ),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: globalEnvironmentKeys.all }),
        queryClient.invalidateQueries({ queryKey: appKeys.all }),
        queryClient.invalidateQueries({ queryKey: databaseKeys.all }),
      ]),
  })
}

export interface MoveResult {
  pending_approval?: DeployApprovalResource
}

export type MoveTarget = 'apps' | 'databases'

export function useMoveEnvironment(kind: MoveTarget) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      environmentId,
      confirm,
    }: {
      name: string
      environmentId: string
      confirm: boolean
    }) =>
      request<MoveResult>(
        `/api/v1/${kind}/${encodeURIComponent(name)}/environment`,
        json('PUT', { environment_id: environmentId, confirm }),
        'move environment',
      ),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: appKeys.all }),
        queryClient.invalidateQueries({ queryKey: databaseKeys.all }),
        queryClient.invalidateQueries({ queryKey: deployApprovalKeys.all }),
        queryClient.invalidateQueries({ queryKey: globalEnvironmentKeys.all }),
      ]),
  })
}
