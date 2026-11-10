// POST /api/v1/apps/{name}/builds with inline_source: build and deploy from
// a pasted Dockerfile and optional context files, no git repo involved.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deployKeys } from './deploys'
import { deployAttemptKeys } from './deployAttempts'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface InlineContextFile {
  path: string
  content: string
}

export interface InlineDockerfileInput {
  dockerfile: string
  files: InlineContextFile[]
}

export interface InlineDockerfileResult {
  id?: string
}

export async function deployInlineDockerfile(
  appName: string,
  input: InlineDockerfileInput,
): Promise<InlineDockerfileResult> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/builds`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        build: { type: 'dockerfile' },
        inline_source: { dockerfile: input.dockerfile, files: input.files },
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `inline build failed: ${res.status}`),
    )
  }
  return (await res.json()) as InlineDockerfileResult
}

export function useDeployInlineDockerfile(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: InlineDockerfileInput) =>
      deployInlineDockerfile(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
    },
  })
}
