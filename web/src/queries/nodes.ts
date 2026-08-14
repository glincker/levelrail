// Query-key factory and fetchers for the /nodes resource, mirroring
// queries/apps.ts and queries/databases.ts exactly: no ad hoc key
// arrays inline in components.
//
// Every route here is AbilityRoot-gated on the backend
// (internal/api/router.go). A session (the only credential type this
// admin dashboard itself ever authenticates with) is implicitly root in
// Phase 1 (there is exactly one human identity, no RBAC yet), so this
// never 403s for the dashboard's own use. useNodeListOptional below
// exists specifically for call sites that might run under a scoped
// write-only API token instead (a future possibility, not true of the
// dashboard today), so a permission failure there degrades gracefully
// rather than breaking the page.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { NodeJoinTokenResponse, NodeResource } from '../types/nodeDetail'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const nodeKeys = {
  all: ['nodes'] as const,
  list: () => [...nodeKeys.all, 'list'] as const,
}

// Fetches every node from the control plane API. GET /api/v1/nodes
// (internal/api/nodes.go's handleListNodes) returns a bare array, no
// cursor, no separate summary projection.
export async function fetchNodes(): Promise<NodeResource[]> {
  const res = await fetch('/api/v1/nodes')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch nodes failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeResource[]
}

export function nodeListQueryOptions() {
  return queryOptions({
    queryKey: nodeKeys.list(),
    queryFn: fetchNodes,
  })
}

// The Nodes route itself: always expected to succeed (an operator
// reaching /nodes at all is, by definition, on a real session), so this
// is the ordinary suspense form, matching useApp/useDatabase.
export function useNodes() {
  return useSuspenseQuery(nodeListQueryOptions())
}

// For call sites that display a node picker as an optional convenience
// inside another flow (the app/database create dialogs): not suspense,
// no retry, and a failure (403 under a scoped token, or any other
// error) is treated as "there is nothing to show" rather than crashing
// the surrounding form. The same graceful-degradation shape
// queries/devMode.ts's own doc comment establishes for an optional
// signal a form's core function must not depend on.
export function useNodeListOptional() {
  return useQuery({ ...nodeListQueryOptions(), retry: false })
}

// POST /api/v1/nodes/join-tokens (internal/api/nodes.go's
// handleCreateNodeJoinToken): mints a one-time enrollment token. Nothing
// to invalidate on success, minting a token doesn't change the node
// list, the node only appears once the agent actually enrolls with it
// (still nothing this dashboard can observe happening in real time as
// of this pass, no polling added here).
export async function createNodeJoinToken(): Promise<NodeJoinTokenResponse> {
  const res = await fetch('/api/v1/nodes/join-tokens', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create node join token failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NodeJoinTokenResponse
}

export function useCreateNodeJoinToken() {
  return useMutation({ mutationFn: createNodeJoinToken })
}

// DELETE /api/v1/nodes/{id} (internal/api/nodes.go's handleDeleteNode).
// Same known gap that handler's own doc comment names: this deletes the
// registry row only, it does not drain or disconnect a real agent
// session (TASKS.md 3.7, not built).
export async function deleteNode(id: string): Promise<void> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete node failed: ${res.status}`),
    )
  }
}

export function useDeleteNode() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteNode,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
    },
  })
}
