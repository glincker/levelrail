import { LOCAL_NODE_VALUE, NO_PROJECT_VALUE } from '../components/PlacementFields'

// resolveSubmittedProjectId converts a create-form's project field value
// into what the create request should actually send: undefined for "no
// project" (the sentinel, or the field simply wasn't touched), the
// project id otherwise. Shared by CreateAppFields and
// CreateDatabaseFields' own onSubmit.
export function resolveSubmittedProjectId(
  project: string | undefined,
): string | undefined {
  return project === NO_PROJECT_VALUE || !project ? undefined : project
}

// resolveSubmittedNodeId converts a create-form's node field value into
// what the create request should actually send: undefined (dropped from
// the request body entirely) unless the operator opened the advanced
// panel, in which case an explicit "local node" choice is sent as '' so
// it's never silently overridden by the server's own auto-placement.
// Shared by CreateAppFields and CreateDatabaseFields' own onSubmit.
export function resolveSubmittedNodeId(
  showAdvanced: boolean,
  nodeId: string,
): string | undefined {
  if (!showAdvanced) return undefined
  return nodeId === LOCAL_NODE_VALUE ? '' : nodeId
}

// autoPlacementToastDescription is the create-success toast's
// description line when simple spread scheduling placed the new
// resource somewhere other than the local node, or undefined otherwise.
// Shared by CreateAppFields and CreateDatabaseFields' own onSuccess.
export function autoPlacementToastDescription(created: {
  auto_placed?: boolean
  node_id?: string
}): string | undefined {
  return created.auto_placed
    ? `Auto-placed on node "${created.node_id}" (simple spread scheduling).`
    : undefined
}
