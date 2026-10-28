// Query hooks for /api/v1/apps/{name}/files: inline config files mounted
// into the app container. Secret files come back redacted (no content).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AppFile {
  path: string
  mode: string
  secret: boolean
  redacted: boolean
  size_bytes: number
  content?: string
}

export interface AppFilesResponse {
  app_name: string
  files: AppFile[]
}

export interface PutAppFileRequest {
  path: string
  content: string
  mode: string
  secret: boolean
}

export interface AppFileMutationOptions {
  restart?: boolean
}

export const appFilesKeys = {
  list: (appName: string) => [...appKeys.detail(appName), 'files'] as const,
}

function filesPath(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/files`
}

function applyQuery(restart: boolean | undefined): string {
  return restart ? 'apply=restart' : ''
}

export async function fetchAppFiles(
  appName: string,
): Promise<AppFilesResponse> {
  const res = await fetch(filesPath(appName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app files failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppFilesResponse
}

export function appFilesQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appFilesKeys.list(appName),
    queryFn: () => fetchAppFiles(appName),
  })
}

export function useAppFiles(appName: string) {
  return useQuery(appFilesQueryOptions(appName))
}

export function usePutAppFile(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<
    void,
    ApiError,
    PutAppFileRequest & AppFileMutationOptions
  >({
    mutationFn: async ({ restart, ...body }) => {
      const qs = applyQuery(restart)
      const res = await fetch(`${filesPath(appName)}${qs ? `?${qs}` : ''}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `save app file failed: ${res.status}`),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: appFilesKeys.list(appName) }),
  })
}

export function useDeleteAppFile(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, { path: string } & AppFileMutationOptions>(
    {
      mutationFn: async ({ path, restart }) => {
        const params = new URLSearchParams({ path })
        if (restart) params.set('apply', 'restart')
        const res = await fetch(`${filesPath(appName)}?${params.toString()}`, {
          method: 'DELETE',
        })
        if (!res.ok) {
          throw new ApiError(
            res.status,
            await readErrorMessage(
              res,
              `delete app file failed: ${res.status}`,
            ),
          )
        }
      },
      onSuccess: () =>
        queryClient.invalidateQueries({ queryKey: appFilesKeys.list(appName) }),
    },
  )
}
