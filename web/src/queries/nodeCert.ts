// Mutations for a node's agent certificate (internal/api/node_cert.go).

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { NodeResource } from '../types/nodeDetail'
import type { NodeReenrollTokenResponse } from '../types/nodeCert'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { nodeKeys } from './nodes'

export async function createNodeReenrollToken(
  id: string,
): Promise<NodeReenrollTokenResponse> {
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(id)}/reenroll-token`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create re-enroll token failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NodeReenrollTokenResponse
}

export function useCreateNodeReenrollToken() {
  return useMutation({ mutationFn: createNodeReenrollToken })
}

export async function revokeNodeCert(id: string): Promise<NodeResource> {
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(id)}/revoke-cert`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `revoke certificate failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeResource
}

export function useRevokeNodeCert() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: revokeNodeCert,
    onSuccess: (updated) => {
      queryClient.setQueryData(nodeKeys.detail(updated.id), updated)
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
    },
  })
}

// The command an operator runs on the node. The control plane address is
// what the node dials, which only the operator knows for sure.
export function reenrollCommand(
  created: NodeReenrollTokenResponse,
  controlPlaneAddr: string,
): string {
  return [
    `APP_CONTROL_PLANE_ADDR=${controlPlaneAddr}`,
    `APP_REENROLL_TOKEN=${created.token}`,
    created.ca_fingerprint
      ? `APP_CA_FINGERPRINT=${created.ca_fingerprint}`
      : null,
    `./${created.agent_binary} reenroll`,
  ]
    .filter(Boolean)
    .join(' ')
}
