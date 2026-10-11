// Compose stacks: status, history, rollback, redeploy, teardown, the
// import preview and manifest template installs (internal/api/routes_compose.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import type { ComposeDeployResponse } from './compose'
import type {
  ComposeDownResult,
  ComposePreview,
  ComposeRevision,
  ComposeStack,
  TemplateInstallInput,
} from '../types/compose'

export const composeKeys = {
  all: ['compose'] as const,
  stack: (appId: string) => [...composeKeys.all, 'stack', appId] as const,
  revisions: (appId: string) =>
    [...composeKeys.all, 'revisions', appId] as const,
  preview: (body: string, profiles: string) =>
    [...composeKeys.all, 'preview', profiles, body] as const,
}

async function request<T>(
  path: string,
  init: RequestInit,
  fallback: string,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
  return (await res.json()) as T
}

const appPath = (appId: string) =>
  `/api/v1/apps/${encodeURIComponent(appId)}/compose`

export function composeStackQueryOptions(appId: string) {
  return queryOptions({
    queryKey: composeKeys.stack(appId),
    queryFn: () =>
      request<ComposeStack>(appPath(appId), {}, 'load compose stack failed'),
    refetchInterval: 5000,
  })
}

// 404 means the app was not deployed from a compose file: the panel hides.
export function useComposeStack(appId: string | undefined) {
  return useQuery({
    ...composeStackQueryOptions(appId ?? ''),
    enabled: !!appId,
    retry: (count, err) =>
      !(err instanceof ApiError && err.status === 404) && count < 2,
  })
}

export function useComposeRevisions(appId: string, enabled: boolean) {
  return useQuery({
    queryKey: composeKeys.revisions(appId),
    queryFn: async () =>
      (
        await request<{ revisions: ComposeRevision[] }>(
          `${appPath(appId)}/revisions`,
          {},
          'load compose revisions failed',
        )
      ).revisions,
    enabled,
  })
}

export function useComposePreview(body: string, profiles: string[]) {
  const joined = profiles.join(',')
  return useQuery({
    queryKey: composeKeys.preview(body, joined),
    queryFn: () =>
      request<ComposePreview>(
        `/api/v1/compose/preview${joined ? `?profiles=${encodeURIComponent(joined)}` : ''}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'text/yaml' },
          body,
        },
        'preview compose failed',
      ),
    enabled: body.trim().length > 0,
    staleTime: 30_000,
  })
}

function useStackMutation<TVars, TResult>(
  fn: (vars: TVars) => Promise<TResult>,
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: composeKeys.all })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: appKeys.all })
    },
  })
}

export function useRedeployCompose(appId: string) {
  return useStackMutation((service?: string) =>
    request<{ restarted: string[] }>(
      `${appPath(appId)}/redeploy`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(service ? { service } : {}),
      },
      'redeploy failed',
    ),
  )
}

export function useRollbackCompose(appId: string) {
  return useStackMutation((revision?: number) =>
    request<{ rolled_back_to: number; deploy: ComposeDeployResponse }>(
      `${appPath(appId)}/rollback`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(revision ? { revision } : {}),
      },
      'rollback failed',
    ),
  )
}

export function useComposeDown(appId: string) {
  return useStackMutation((removeVolumes: boolean) =>
    request<ComposeDownResult>(
      `${appPath(appId)}${removeVolumes ? '?volumes=true' : ''}`,
      { method: 'DELETE' },
      'tear down failed',
    ),
  )
}

export function useInstallTemplate(templateId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: TemplateInstallInput) =>
      request<ComposeDeployResponse>(
        `/api/v1/service-templates/${encodeURIComponent(templateId)}/install`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        },
        'install failed',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: composeKeys.all })
    },
  })
}
