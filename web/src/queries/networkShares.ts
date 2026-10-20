// Query-key factory and fetchers for the /network-shares resource
// (internal/api/network_shares.go), following the same shared-
// queryOptions pattern queries/registryCredentials.ts already
// established.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  CreateNetworkShareRequest,
  NetworkShare,
  UpdateNetworkShareRequest,
} from '../types/networkShare'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const networkShareKeys = {
  all: ['network-shares'] as const,
  list: () => [...networkShareKeys.all, 'list'] as const,
}

// GET /api/v1/network-shares (handleListNetworkShares): no 501 case,
// just lists whatever rows already exist.
export async function fetchNetworkShares(): Promise<NetworkShare[]> {
  const res = await fetch('/api/v1/network-shares')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch network shares failed: ${res.status}`),
    )
  }
  return (await res.json()) as NetworkShare[]
}

export function networkShareListQueryOptions() {
  return queryOptions({
    queryKey: networkShareKeys.list(),
    queryFn: fetchNetworkShares,
  })
}

export function useNetworkShares() {
  return useSuspenseQuery(networkShareListQueryOptions())
}

// Non-suspending variant for a volume-attach form that wants the share
// list as a supplementary signal without making an unrelated tree's
// whole Suspense boundary wait on it, the same reasoning
// useRegistryCredentialsOptional already uses.
export function useNetworkSharesOptional() {
  return useQuery(networkShareListQueryOptions())
}

// POST /api/v1/network-shares (handleCreateNetworkShare). 501 means a
// cifs share was submitted without a master key configured on this
// control plane, the same server-configuration-gap case
// createRegistryCredential carries for the identical reason.
export async function createNetworkShare(
  req: CreateNetworkShareRequest,
): Promise<NetworkShare> {
  const res = await fetch('/api/v1/network-shares', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'CIFS/SMB network shares require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create network share failed: ${res.status}`),
    )
  }
  return (await res.json()) as NetworkShare
}

export function useCreateNetworkShare() {
  const queryClient = useQueryClient()
  return useMutation<NetworkShare, ApiError, CreateNetworkShareRequest>({
    mutationFn: createNetworkShare,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: networkShareKeys.list() })
    },
  })
}

// PUT /api/v1/network-shares/{id} (handleUpdateNetworkShare).
export async function updateNetworkShare(
  id: string,
  req: UpdateNetworkShareRequest,
): Promise<NetworkShare> {
  const res = await fetch(`/api/v1/network-shares/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'CIFS/SMB network shares require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update network share failed: ${res.status}`),
    )
  }
  return (await res.json()) as NetworkShare
}

export function useUpdateNetworkShare() {
  const queryClient = useQueryClient()
  return useMutation<
    NetworkShare,
    ApiError,
    { id: string; req: UpdateNetworkShareRequest }
  >({
    mutationFn: ({ id, req }) => updateNetworkShare(id, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: networkShareKeys.list() })
    },
  })
}

// DELETE /api/v1/network-shares/{id} (handleDeleteNetworkShare). 204 on
// success, no body to parse.
export async function deleteNetworkShare(id: string): Promise<void> {
  const res = await fetch(`/api/v1/network-shares/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `delete network share failed: ${res.status}`),
  )
}

export function useDeleteNetworkShare() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deleteNetworkShare,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: networkShareKeys.list() })
    },
  })
}

// POST /api/v1/network-shares/{id}/test (handleTestNetworkShare): dials
// the share's host on its protocol's standard port. 204 on success, 502
// with a specific reason on failure, the same shape
// testRegistryCredential already establishes for its own on-demand
// verification.
export async function testNetworkShare(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/network-shares/${encodeURIComponent(id)}/test`,
    { method: 'POST' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `test network share failed: ${res.status}`),
  )
}

export function useTestNetworkShare() {
  return useMutation<void, ApiError, string>({
    mutationFn: testNetworkShare,
  })
}
