// Query-key factory and fetchers for /api/v1/iam/policies
// (the IAM policy engine's HTTP surface, already shipped with a CLI at
// levelrail-cli iam policies create/list/get/update/delete/attach/detach/
// attachments but no dashboard UI until this file). Session-cookie
// authenticated like every other settings page, mirrors queries/tokens.ts
// and queries/deviceAuth.ts's shared conventions: one queryOptions() per
// resource, useSuspenseQuery for the list, useMutation + invalidate for
// writes.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type PrincipalType = 'user' | 'token'

export interface PolicyDocument {
  Statement: {
    Effect: 'Allow' | 'Deny'
    Action: string[]
    Resource: string[]
  }[]
}

export interface PolicyResource {
  id: string
  name: string
  description: string
  document: PolicyDocument
  created_at: string
  updated_at: string
}

export interface PolicyAttachment {
  id: string
  policy_id: string
  principal_type: PrincipalType
  principal_id: string
  created_at: string
}

export interface PolicyWriteRequest {
  name: string
  description: string
  document: PolicyDocument
}

export const iamPolicyKeys = {
  all: ['iam-policies'] as const,
  list: () => [...iamPolicyKeys.all, 'list'] as const,
  attachments: (policyId: string) =>
    [...iamPolicyKeys.all, 'attachments', policyId] as const,
}

export async function fetchPolicies(): Promise<PolicyResource[]> {
  const res = await fetch('/api/v1/iam/policies')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch policies failed: ${res.status}`),
    )
  }
  return (await res.json()) as PolicyResource[]
}

export function policyListQueryOptions() {
  return queryOptions({
    queryKey: iamPolicyKeys.list(),
    queryFn: fetchPolicies,
  })
}

export function usePolicies() {
  return useSuspenseQuery(policyListQueryOptions())
}

export async function createPolicy(
  req: PolicyWriteRequest,
): Promise<PolicyResource> {
  const res = await fetch('/api/v1/iam/policies', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create policy failed: ${res.status}`),
    )
  }
  return (await res.json()) as PolicyResource
}

export function useCreatePolicy() {
  const queryClient = useQueryClient()
  return useMutation<PolicyResource, ApiError, PolicyWriteRequest>({
    mutationFn: createPolicy,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: iamPolicyKeys.list() })
    },
  })
}

export async function updatePolicy(
  id: string,
  req: PolicyWriteRequest,
): Promise<PolicyResource> {
  const res = await fetch(`/api/v1/iam/policies/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update policy failed: ${res.status}`),
    )
  }
  return (await res.json()) as PolicyResource
}

export function useUpdatePolicy() {
  const queryClient = useQueryClient()
  return useMutation<
    PolicyResource,
    ApiError,
    { id: string; req: PolicyWriteRequest }
  >({
    mutationFn: ({ id, req }) => updatePolicy(id, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: iamPolicyKeys.list() })
    },
  })
}

export async function deletePolicy(id: string): Promise<void> {
  const res = await fetch(`/api/v1/iam/policies/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete policy failed: ${res.status}`),
    )
  }
}

export function useDeletePolicy() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deletePolicy,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: iamPolicyKeys.list() })
    },
  })
}

export async function fetchPolicyAttachments(
  policyId: string,
): Promise<PolicyAttachment[]> {
  const res = await fetch(
    `/api/v1/iam/policies/${encodeURIComponent(policyId)}/attachments`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch attachments failed: ${res.status}`),
    )
  }
  return (await res.json()) as PolicyAttachment[]
}

export function policyAttachmentsQueryOptions(policyId: string) {
  return queryOptions({
    queryKey: iamPolicyKeys.attachments(policyId),
    queryFn: () => fetchPolicyAttachments(policyId),
    enabled: policyId.length > 0,
  })
}

export function usePolicyAttachments(policyId: string) {
  return useSuspenseQuery(policyAttachmentsQueryOptions(policyId))
}

export interface AttachPrincipalRequest {
  policyId: string
  principal_type: PrincipalType
  principal_id: string
}

export async function attachPrincipal(
  req: AttachPrincipalRequest,
): Promise<void> {
  const res = await fetch(
    `/api/v1/iam/policies/${encodeURIComponent(req.policyId)}/attachments`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        principal_type: req.principal_type,
        principal_id: req.principal_id,
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `attach principal failed: ${res.status}`),
    )
  }
}

export function useAttachPrincipal() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, AttachPrincipalRequest>({
    mutationFn: attachPrincipal,
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({
        queryKey: iamPolicyKeys.attachments(variables.policyId),
      })
    },
  })
}

export interface DetachPrincipalRequest {
  policyId: string
  principal_type: PrincipalType
  principal_id: string
}

export async function detachPrincipal(
  req: DetachPrincipalRequest,
): Promise<void> {
  const res = await fetch(
    `/api/v1/iam/policies/${encodeURIComponent(req.policyId)}/attachments/${encodeURIComponent(req.principal_type)}/${encodeURIComponent(req.principal_id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `detach principal failed: ${res.status}`),
    )
  }
}

export function useDetachPrincipal() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, DetachPrincipalRequest>({
    mutationFn: detachPrincipal,
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({
        queryKey: iamPolicyKeys.attachments(variables.policyId),
      })
    },
  })
}
