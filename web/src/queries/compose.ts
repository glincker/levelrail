// Query-key-free mutation for POST /api/v1/apps/{name}/compose
// (internal/api/apps_compose.go's handleDeployCompose): unlike every
// other fetcher in this directory, the request body is a raw
// compose.yaml document, not JSON, so this intentionally does not
// reuse createApp's JSON.stringify pattern.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { AppDetail } from '../types/appDetail'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'

// Mirrors internal/api/apps_compose.go's composeNoticeResult: a compose
// keyword that parsed but doesn't behave the way it would under real
// Docker Compose (restart:, networks:), surfaced instead of silently
// dropped. "note" is informational, "warning" flags a real semantic gap.
export interface ComposeNotice {
  level: 'note' | 'warning'
  message: string
}

// Mirrors internal/api/apps_compose.go's composeDeployResponse: each
// entry in `services` is the same appResource wire shape AppDetail
// already types for every other app read/write endpoint.
export interface ComposeDeployResponse {
  app_id: string
  services: AppDetail[]
  notices?: ComposeNotice[]
}

export async function deployCompose(
  name: string,
  composeYaml: string,
): Promise<ComposeDeployResponse> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/compose`, {
    method: 'POST',
    headers: { 'Content-Type': 'text/yaml' },
    body: composeYaml,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `deploy compose failed: ${res.status}`),
    )
  }
  return (await res.json()) as ComposeDeployResponse
}

// Writes each returned service straight into its own app detail cache
// entry, the same setQueryData-on-success shape useCreateApp already
// establishes, since every service here is itself a full AppDetail.
export function useDeployCompose() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      composeYaml,
    }: {
      name: string
      composeYaml: string
    }) => deployCompose(name, composeYaml),
    onSuccess: (result) => {
      for (const service of result.services) {
        queryClient.setQueryData(appKeys.detail(service.name), service)
      }
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
