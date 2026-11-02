import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DeployUrlCopiesMode = 'off' | 'private' | 'public'

// Matches internal/api's deployURLCopiesResource.
export interface DeployUrlCopies {
  mode: DeployUrlCopiesMode
  source: 'stored' | 'env' | 'default'
  default: DeployUrlCopiesMode
  modes: DeployUrlCopiesMode[]
}

export const deployUrlCopiesKeys = {
  all: ['deploy-url-copies'] as const,
}

const ENDPOINT = '/api/v1/settings/deploy-url-copies'

async function fetchDeployUrlCopies(): Promise<DeployUrlCopies> {
  const res = await fetch(ENDPOINT)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch deploy url copies failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DeployUrlCopies
}

export function deployUrlCopiesQueryOptions() {
  return queryOptions({
    queryKey: deployUrlCopiesKeys.all,
    queryFn: fetchDeployUrlCopies,
    staleTime: 10_000,
  })
}

export function useDeployUrlCopies() {
  return useQuery(deployUrlCopiesQueryOptions())
}

// An empty mode clears the stored value so the env seed or default applies.
async function putDeployUrlCopies(
  mode: DeployUrlCopiesMode | '',
): Promise<DeployUrlCopies> {
  const res = await fetch(ENDPOINT, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mode }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update deploy url copies failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DeployUrlCopies
}

export function useUpdateDeployUrlCopies() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: putDeployUrlCopies,
    onSuccess: (data) => {
      queryClient.setQueryData(deployUrlCopiesKeys.all, data)
    },
  })
}
