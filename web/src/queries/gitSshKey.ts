// Fetchers for GET/POST/DELETE /api/v1/apps/{name}/git-source/ssh-key
// (internal/api/git_ssh_key.go). Only the public half ever comes back.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface GitSshKeyResource {
  public_key: string
  fingerprint: string
  host_fingerprint?: string
  host_key_pinned: boolean
}

export type SetGitSshKeyRequest =
  | { host_key: string; trust_on_first_use?: false }
  | { trust_on_first_use: true; host_key?: string }

export const gitSshKeyKeys = {
  detail: (name: string) => ['git-sources', 'ssh-key', name] as const,
}

function url(name: string): string {
  return `/api/v1/apps/${encodeURIComponent(name)}/git-source/ssh-key`
}

// 404 resolves to null: "no key yet" is the normal state, not an error.
export async function fetchGitSshKey(
  name: string,
): Promise<GitSshKeyResource | null> {
  const res = await fetch(url(name))
  if (res.status === 404) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch ssh key failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitSshKeyResource
}

export function gitSshKeyQueryOptions(name: string) {
  return queryOptions({
    queryKey: gitSshKeyKeys.detail(name),
    queryFn: () => fetchGitSshKey(name),
    retry: false,
  })
}

export function useGitSshKey(name: string) {
  return useQuery(gitSshKeyQueryOptions(name))
}

export function useSetGitSshKey(name: string) {
  const queryClient = useQueryClient()
  return useMutation<GitSshKeyResource, ApiError, SetGitSshKeyRequest>({
    mutationFn: async (req) => {
      const res = await fetch(url(name), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `set ssh key failed: ${res.status}`),
        )
      }
      return (await res.json()) as GitSshKeyResource
    },
    onSuccess: (resource) => {
      queryClient.setQueryData(gitSshKeyKeys.detail(name), resource)
    },
  })
}

export function useDeleteGitSshKey(name: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(url(name), { method: 'DELETE' })
      if (res.status === 204 || res.ok) return
      throw new ApiError(
        res.status,
        await readErrorMessage(res, `remove ssh key failed: ${res.status}`),
      )
    },
    onSuccess: () => {
      queryClient.setQueryData(gitSshKeyKeys.detail(name), null)
    },
  })
}
