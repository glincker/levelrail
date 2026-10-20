// Mutation for PUT /api/v1/apps/{name}/volumes
// (internal/api/apps_volumes_attach.go): attach or remove a named
// Docker volume on an app that already exists, without hand-editing
// app.yaml and redeploying. Mirrors queries/appEgress.ts's shape: a
// single full-replace PUT, no separate GET since AppDetail.volumes
// (queries/apps.ts) already carries the current list.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AppVolume } from '../types/appDetail'

function appVolumesPath(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes`
}

// Mirrors internal/api's setAppVolumesResponse wire shape exactly.
interface SetAppVolumesResponse {
  name: string
  volumes?: AppVolume[]
}

async function setAppVolumes(
  appName: string,
  volumes: AppVolume[],
): Promise<SetAppVolumesResponse> {
  const res = await fetch(appVolumesPath(appName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ volumes }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set app volumes failed: ${res.status}`),
    )
  }
  return (await res.json()) as SetAppVolumesResponse
}

// Writes the server's response volumes straight into the app detail
// query's cache, the same "PUT response already is the new canonical
// AppDetail slice" shape useUpdateApp's own onSuccess uses, rather than
// invalidating and refetching the whole app.
export function useSetAppVolumes(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<AppVolume[], ApiError, AppVolume[]>({
    mutationFn: async (volumes) => {
      const res = await setAppVolumes(appName, volumes)
      return res.volumes ?? []
    },
    onSuccess: (volumes) => {
      queryClient.setQueryData(appKeys.detail(appName), (prev: unknown) => {
        if (!prev || typeof prev !== 'object') return prev
        return { ...prev, volumes }
      })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
