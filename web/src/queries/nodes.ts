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
import type {
  DrainNodeResponse,
  NodeJoinTokenResponse,
  NodeResource,
} from '../types/nodeDetail'
import type { ReconcileCondition } from '../types/deploy'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { databaseKeys } from './databases'

export const nodeKeys = {
  all: ['nodes'] as const,
  list: () => [...nodeKeys.all, 'list'] as const,
  detail: (id: string) => [...nodeKeys.all, 'detail', id] as const,
  health: (id: string) => [...nodeKeys.detail(id), 'health'] as const,
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

// Fetches a single node for the detail route, GET /api/v1/nodes/{id}
// (internal/api/nodes.go's handleGetNode).
export async function fetchNode(id: string): Promise<NodeResource> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(id)}`)
  if (res.status === 404) {
    throw new ApiError(404, `node not found: ${id}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch node failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeResource
}

export function nodeDetailQueryOptions(id: string) {
  return queryOptions({
    queryKey: nodeKeys.detail(id),
    queryFn: () => fetchNode(id),
  })
}

// Thin wrapper the detail route's component uses, mirroring useDatabase.
export function useNode(id: string) {
  return useSuspenseQuery(nodeDetailQueryOptions(id))
}

// GET /api/v1/nodes/{id}/health (internal/api/nodes.go's
// handleGetNodeHealth): the node-health controller's stored conditions,
// same ReconcileCondition[] shape ConditionsPanel already renders for
// apps and databases. A node whose health controller hasn't reconciled
// yet gets an empty list, not an error, per that handler's own doc
// comment, so this normalizes a null body the same way
// fetchDatabaseStatus already does.
export async function fetchNodeHealth(
  id: string,
): Promise<ReconcileCondition[]> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(id)}/health`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch node health failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as ReconcileCondition[] | null
  return body ?? []
}

export function nodeHealthQueryOptions(id: string) {
  return queryOptions({
    queryKey: nodeKeys.health(id),
    queryFn: () => fetchNodeHealth(id),
  })
}

export function useNodeHealth(id: string) {
  return useSuspenseQuery(nodeHealthQueryOptions(id))
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

// Shared by cordonNode/uncordonNode below: both POST
// /api/v1/nodes/{id}/cordon and /uncordon (internal/api/nodes.go's
// handleCordonNode/handleUncordonNode) take no body and return the
// updated nodeResource, idempotent either direction per those handlers'
// own doc comments.
async function setNodeCordon(
  id: string,
  cordon: boolean,
): Promise<NodeResource> {
  const action = cordon ? 'cordon' : 'uncordon'
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(id)}/${action}`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${action} node failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeResource
}

export function cordonNode(id: string): Promise<NodeResource> {
  return setNodeCordon(id, true)
}

export function uncordonNode(id: string): Promise<NodeResource> {
  return setNodeCordon(id, false)
}

// Both mutations write the returned nodeResource straight into the
// detail query's cache (mirroring useSetDatabaseNode's own
// setQueryData-on-success shape) and invalidate the list, since
// schedulable is shown in both places.
export function useCordonNode() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: cordonNode,
    onSuccess: (updated) => {
      queryClient.setQueryData(nodeKeys.detail(updated.id), updated)
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
    },
  })
}

export function useUncordonNode() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: uncordonNode,
    onSuccess: (updated) => {
      queryClient.setQueryData(nodeKeys.detail(updated.id), updated)
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
    },
  })
}

// POST /api/v1/nodes/{id}/drain?target_node_id=<id-or-empty>
// (internal/api/nodes.go's handleDrainNode): moves every app and
// database placed on id to targetNodeId (empty string is the
// local/control-plane sentinel, the same convention setDatabaseNode's
// own node_id already uses). Returns the full DrainNodeResponse rather
// than throwing on a 207: a partial failure is real data the caller
// must render (which resources moved, which didn't), not an exception,
// per drainNodeResponse's own doc comment in the backend. Only a
// genuine transport/5xx failure (res.ok false and status is neither 200
// nor 207) throws.
export async function drainNode({
  id,
  targetNodeId,
}: {
  id: string
  targetNodeId: string
}): Promise<DrainNodeResponse> {
  const params = new URLSearchParams({ target_node_id: targetNodeId })
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(id)}/drain?${params.toString()}`,
    { method: 'POST' },
  )
  if (!res.ok && res.status !== 207) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `drain node failed: ${res.status}`),
    )
  }
  return (await res.json()) as DrainNodeResponse
}

// Draining moves apps and databases onto a different node, so their own
// list/detail queries go stale too, not just this node's: invalidating
// appKeys.list()/databaseKeys.list() here (rather than leaving that to
// whichever route the operator happens to visit next) keeps the apps and
// databases pages honest about node placement immediately after a drain,
// the same cross-resource invalidation reasoning useSetDatabaseNode
// already applies to its own single-resource move.
export function useDrainNode() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: drainNode,
    onSuccess: (_result, { id }) => {
      void queryClient.invalidateQueries({ queryKey: nodeKeys.detail(id) })
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}

// PUT /api/v1/nodes/{id}/workloads (internal/api/nodes.go's
// handleSetNodeWorkloads): both fields are always required together,
// matching setNodeWorkloadsRequest's own doc comment that this is a
// full replace, never an independent per-field patch.
export async function setNodeWorkloads({
  id,
  acceptsAppWorkloads,
  acceptsBuildWorkloads,
}: {
  id: string
  acceptsAppWorkloads: boolean
  acceptsBuildWorkloads: boolean
}): Promise<NodeResource> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(id)}/workloads`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      accepts_app_workloads: acceptsAppWorkloads,
      accepts_build_workloads: acceptsBuildWorkloads,
    }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set node workloads failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeResource
}

export function useSetNodeWorkloads() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: setNodeWorkloads,
    onSuccess: (updated) => {
      queryClient.setQueryData(nodeKeys.detail(updated.id), updated)
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
    },
  })
}
