import { toast } from '@/components/ui/toast'
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

// buildCreateResourceSuccessHandler returns the onSuccess callback every
// create-resource form's mutate() call needs: clear the draft, notify
// the caller, toast the outcome (including autoPlacementToastDescription's
// own auto-placement note when relevant), then hand the new resource's
// name to onNavigate. Deliberately takes onNavigate rather than a
// generic "navigate" function: TanStack Router's own navigate is typed
// against the app's generated route tree (a literal "to" string per
// route), which a shared, resource-agnostic helper can't reproduce
// without losing that type safety, so each caller keeps its own
// `navigate({ to: '/apps/$name', ... })` call inline instead. Shared by
// CreateAppFields and CreateDatabaseFields' own onSuccess.
export function buildCreateResourceSuccessHandler<
  T extends { name: string; auto_placed?: boolean; node_id?: string },
>(opts: {
  resourceLabel: string
  clearDraft: () => void
  onCreated: () => void
  onNavigate: (name: string) => void
}) {
  return (created: T) => {
    opts.clearDraft()
    opts.onCreated()
    toast.add({
      title: `${opts.resourceLabel} "${created.name}" created.`,
      description: autoPlacementToastDescription(created),
      type: 'success',
    })
    opts.onNavigate(created.name)
  }
}
