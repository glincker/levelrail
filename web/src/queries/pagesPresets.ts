import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { staticSitesKeys } from './staticSites'

export interface PagesPreset {
  id: string
  name: string
  description: string
  compression: string[]
}

interface PresetChoice {
  name: string
  preset: string
}

const choiceKey = (name: string) =>
  [...staticSitesKeys.all, 'preset', name] as const

async function readJson<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function usePagesPresets() {
  return useQuery(
    queryOptions({
      queryKey: [...staticSitesKeys.all, 'presets'] as const,
      queryFn: async (): Promise<PagesPreset[]> => {
        const body = await readJson<{ presets: PagesPreset[] }>(
          await fetch('/api/v1/pages/presets'),
          'presets',
        )
        return body.presets
      },
      staleTime: 600_000,
      retry: false,
    }),
  )
}

export function useSitePreset(name: string) {
  return useQuery(
    queryOptions({
      queryKey: choiceKey(name),
      queryFn: async (): Promise<PresetChoice> =>
        readJson<PresetChoice>(
          await fetch(
            `/api/v1/static-sites/${encodeURIComponent(name)}/preset`,
          ),
          'preset',
        ),
      retry: false,
    }),
  )
}

export function useSetSitePreset(name: string) {
  const qc = useQueryClient()
  return useMutation<PresetChoice, ApiError, string>({
    mutationFn: async (preset) =>
      readJson<PresetChoice>(
        await fetch(`/api/v1/static-sites/${encodeURIComponent(name)}/preset`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ preset }),
        }),
        'set preset',
      ),
    onSuccess: (data) => {
      qc.setQueryData(choiceKey(name), data)
    },
  })
}
