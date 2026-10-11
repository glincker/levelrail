// Detection and build-settings fetchers for a connected git source:
// POST /api/v1/apps/{name}/git-source/detect and
// PUT /api/v1/apps/{name}/git-source/build (internal/api/git_build_settings.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  GitBuildDetection,
  GitSourceResource,
  SetGitSourceBuildRequest,
} from '../types/gitSource'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { gitSourceKeys } from './gitSources'

export const gitBuildKeys = {
  detect: (name: string, branch: string) =>
    [...gitSourceKeys.all, 'detect', name, branch] as const,
}

export async function detectGitBuild(
  name: string,
  branch?: string,
): Promise<GitBuildDetection> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/git-source/detect`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(branch ? { branch } : {}),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `detect build settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitBuildDetection
}

export function gitBuildDetectOptions(
  name: string,
  branch: string,
  enabled: boolean,
) {
  return queryOptions({
    queryKey: gitBuildKeys.detect(name, branch),
    queryFn: () => detectGitBuild(name, branch),
    enabled,
    staleTime: 60_000,
    retry: false,
  })
}

// useGitBuildDetection reads the provider's file tree on demand. enabled is
// false until the caller needs it, so a connected card never lists a repo
// that nobody is looking at.
export function useGitBuildDetection(
  name: string,
  branch: string,
  enabled: boolean,
) {
  return useQuery(gitBuildDetectOptions(name, branch, enabled))
}

export async function setGitBuild(
  name: string,
  req: SetGitSourceBuildRequest,
): Promise<GitSourceResource> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/git-source/build`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `save build settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSourceResource
}

export function useSetGitBuild(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: SetGitSourceBuildRequest) => setGitBuild(name, req),
    onSuccess: (resource) => {
      queryClient.setQueryData(gitSourceKeys.detail(name), resource)
      void queryClient.invalidateQueries({
        queryKey: [...gitSourceKeys.all, 'detect', name],
      })
    },
  })
}
