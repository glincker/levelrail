import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { readErrorMessage } from '../../lib/apiError'
import type { BuildSettings, BuildSettingsPayload } from './buildSettingsForm'

const key = (name: string) => ['build-settings', name] as const

function url(name: string) {
  return `/api/v1/apps/${encodeURIComponent(name)}/build-settings`
}

export function useBuildSettings(name: string) {
  return useQuery({
    queryKey: key(name),
    queryFn: async (): Promise<BuildSettings> => {
      const res = await fetch(url(name))
      if (!res.ok)
        throw new Error(
          await readErrorMessage(res, 'could not load build settings'),
        )
      return (await res.json()) as BuildSettings
    },
  })
}

export function useSaveBuildSettings(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: BuildSettingsPayload): Promise<BuildSettings> => {
      const res = await fetch(url(name), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok)
        throw new Error(
          await readErrorMessage(res, 'could not save build settings'),
        )
      return (await res.json()) as BuildSettings
    },
    onSuccess: (data) => {
      qc.setQueryData(key(name), data)
      void qc.invalidateQueries({ queryKey: ['git-sources'] })
    },
  })
}
