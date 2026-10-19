import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DeployCompare } from '../types/deployCompare'
import { appKeys } from './apps'

export async function fetchPendingDeployDiff(
  appName: string,
  image: string,
  signal?: AbortSignal,
): Promise<DeployCompare | null> {
  const params = new URLSearchParams({ pending: 'true' })
  if (image) {
    params.set('image', image)
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys/compare?${params.toString()}`,
    { signal },
  )
  if (res.status === 404) {
    return null
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch pending changes failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DeployCompare
}

export function usePendingDeployDiff(appName: string, image: string) {
  return useQuery({
    queryKey: [...appKeys.detail(appName), 'deploys', 'pending-diff', image],
    queryFn: ({ signal }) => fetchPendingDeployDiff(appName, image, signal),
    staleTime: 10_000,
  })
}
