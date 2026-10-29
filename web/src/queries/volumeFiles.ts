// Fetchers for the app volume file endpoints (internal/api/app_volume_files.go).
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type VolumeFileEntry = {
  name: string
  path: string
  type: 'file' | 'dir' | 'symlink' | 'other'
  size: number
  mode: string
  mod_time: string
}

export type VolumeFileListing = {
  path: string
  entries: VolumeFileEntry[]
  truncated: boolean
}

const base = (app: string, volume: string) =>
  `/api/v1/apps/${encodeURIComponent(app)}/volumes/${encodeURIComponent(volume)}/files`

export const volumeFileKeys = {
  all: (app: string, volume: string) => ['volume-files', app, volume] as const,
  dir: (app: string, volume: string, dir: string) =>
    ['volume-files', app, volume, dir] as const,
}

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function volumeFilesQueryOptions(
  app: string,
  volume: string,
  dir: string,
) {
  return queryOptions({
    queryKey: volumeFileKeys.dir(app, volume, dir),
    queryFn: async () => {
      const res = await fetch(
        `${base(app, volume)}?path=${encodeURIComponent(dir)}`,
      )
      if (!res.ok) return fail(res, 'list volume files')
      return (await res.json()) as VolumeFileListing
    },
  })
}

export function useVolumeFiles(app: string, volume: string, dir: string) {
  return useQuery({
    ...volumeFilesQueryOptions(app, volume, dir),
    retry: false,
  })
}

export function volumeFileDownloadURL(
  app: string,
  volume: string,
  path: string,
) {
  return `${base(app, volume)}/content?path=${encodeURIComponent(path)}`
}

export function useUploadVolumeFile(app: string, volume: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (vars: { path: string; file: File }) => {
      const res = await fetch(
        `${base(app, volume)}/content?path=${encodeURIComponent(vars.path)}`,
        { method: 'PUT', body: vars.file },
      )
      if (!res.ok) return fail(res, 'upload volume file')
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: volumeFileKeys.all(app, volume) }),
  })
}

export function useDeleteVolumeFile(app: string, volume: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (path: string) => {
      const res = await fetch(volumeFileDownloadURL(app, volume, path), {
        method: 'DELETE',
      })
      if (!res.ok) return fail(res, 'delete volume file')
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: volumeFileKeys.all(app, volume) }),
  })
}
